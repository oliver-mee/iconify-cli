// Package iconindex builds and queries the local mirror of the Iconify corpus.
//
// The Iconify API cannot answer corpus-shaped questions: /collections returns
// one object keyed by prefix, and icon names live only inside per-prefix
// /collection responses. Mirroring both into SQLite is what makes
// coverage, migration, and audit queries possible at all.
package iconindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Fetcher is the subset of the generated API client this package needs.
type Fetcher interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// SetInfo is one row of /collections.
type SetInfo struct {
	Prefix   string
	Name     string
	Total    int
	Category string
	Author   string
	License  string
	SPDX     string
	URL      string
	Palette  bool
	Height   int
}

// collectionsPayload mirrors /collections: prefix -> info block.
type infoBlock struct {
	Name     string `json:"name"`
	Total    int    `json:"total"`
	Category string `json:"category"`
	Palette  bool   `json:"palette"`
	Height   any    `json:"height"`
	Author   struct {
		Name string `json:"name"`
	} `json:"author"`
	License struct {
		Title string `json:"title"`
		SPDX  string `json:"spdx"`
		URL   string `json:"url"`
	} `json:"license"`
}

// collectionPayload mirrors /collection?prefix=X.
type collectionPayload struct {
	Prefix        string              `json:"prefix"`
	Total         int                 `json:"total"`
	Uncategorized []string            `json:"uncategorized"`
	Categories    map[string][]string `json:"categories"`
	Hidden        []string            `json:"hidden"`
	Aliases       map[string]string   `json:"aliases"`
}

// IconRow is one icon in the local mirror.
type IconRow struct {
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	AliasOf  string `json:"alias_of,omitempty"`
}

// FullName renders the canonical "prefix:name" form.
func (r IconRow) FullName() string { return r.Prefix + ":" + r.Name }

// ParseSets converts a /collections payload into sorted SetInfo rows.
func ParseSets(raw json.RawMessage) ([]SetInfo, error) {
	var m map[string]infoBlock
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing collections: %w", err)
	}
	out := make([]SetInfo, 0, len(m))
	for prefix, b := range m {
		out = append(out, SetInfo{
			Prefix:   prefix,
			Name:     b.Name,
			Total:    b.Total,
			Category: b.Category,
			Author:   b.Author.Name,
			License:  b.License.Title,
			SPDX:     b.License.SPDX,
			URL:      b.License.URL,
			Palette:  b.Palette,
			Height:   coerceHeight(b.Height),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })
	return out, nil
}

// coerceHeight accepts the number-or-string-or-array shapes /collections uses
// for height; anything unrecognised becomes 0 rather than failing the parse.
func coerceHeight(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		var n int
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n
		}
	case []any:
		if len(t) > 0 {
			return coerceHeight(t[0])
		}
	}
	return 0
}

// ParseCollection flattens a /collection payload into icon rows. Hidden icons
// are retained and flagged rather than dropped: the API keeps removed icons
// visible-but-hidden so existing apps do not break, and `audit` needs to tell
// a caller that a name they use has been retired.
func ParseCollection(prefix string, raw json.RawMessage) ([]IconRow, error) {
	var p collectionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parsing collection %s: %w", prefix, err)
	}
	seen := make(map[string]int, p.Total+len(p.Aliases))
	rows := make([]IconRow, 0, p.Total+len(p.Aliases))
	add := func(name, category string, hidden bool, aliasOf string) {
		if name == "" {
			return
		}
		if idx, ok := seen[name]; ok {
			// A name can appear in a category and in hidden; keep the richer row.
			if category != "" && rows[idx].Category == "" {
				rows[idx].Category = category
			}
			if hidden {
				rows[idx].Hidden = true
			}
			return
		}
		seen[name] = len(rows)
		rows = append(rows, IconRow{Prefix: prefix, Name: name, Category: category, Hidden: hidden, AliasOf: aliasOf})
	}
	for cat, names := range p.Categories {
		for _, n := range names {
			add(n, cat, false, "")
		}
	}
	for _, n := range p.Uncategorized {
		add(n, "", false, "")
	}
	for _, n := range p.Hidden {
		add(n, "", true, "")
	}
	for alias, parent := range p.Aliases {
		add(alias, "", false, parent)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

// ParseLastModified flattens /last-modified into prefix -> epoch seconds.
func ParseLastModified(raw json.RawMessage) (map[string]int64, error) {
	var p struct {
		LastModified map[string]int64 `json:"lastModified"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parsing last-modified: %w", err)
	}
	if p.LastModified == nil {
		p.LastModified = map[string]int64{}
	}
	return p.LastModified, nil
}

// Progress reports index build progress to the caller.
type Progress func(done, total int, prefix string)

// BuildOptions tunes an index build.
type BuildOptions struct {
	Concurrency int
	Only        []string // restrict to these prefixes
	Force       bool     // reindex sets whose lastModified has not advanced
	OnProgress  Progress
}

// Result summarises an index build.
type Result struct {
	Sets       int      `json:"sets"`
	Icons      int      `json:"icons"`
	Refreshed  []string `json:"refreshed"`
	Skipped    int      `json:"skipped"`
	Failed     []string `json:"failed,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// Build mirrors the corpus into db. It fetches /collections once, consults
// /last-modified to decide which sets actually changed, and only pulls
// /collection for those, so a repeat run costs two requests when nothing moved.
func Build(ctx context.Context, api Fetcher, db *sql.DB, opts BuildOptions) (*Result, error) {
	started := time.Now()
	if opts.Concurrency <= 0 {
		opts.Concurrency = 8
	}
	res := &Result{Refreshed: []string{}, Failed: []string{}}

	rawSets, err := api.Get(ctx, "/collections", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching collections: %w", err)
	}
	sets, err := ParseSets(rawSets)
	if err != nil {
		return nil, err
	}
	if len(opts.Only) > 0 {
		keep := make(map[string]bool, len(opts.Only))
		for _, p := range opts.Only {
			keep[p] = true
		}
		filtered := sets[:0]
		for _, s := range sets {
			if keep[s.Prefix] {
				filtered = append(filtered, s)
			}
		}
		sets = filtered
	}
	if len(sets) == 0 {
		return nil, fmt.Errorf("no icon sets matched")
	}

	lastMod := map[string]int64{}
	if rawLM, lmErr := api.Get(ctx, "/last-modified", nil); lmErr == nil {
		if parsed, pErr := ParseLastModified(rawLM); pErr == nil {
			lastMod = parsed
		}
	}
	stored, err := storedLastModified(ctx, db)
	if err != nil {
		return nil, err
	}

	type job struct {
		set SetInfo
		lm  int64
	}
	var todo []job
	for _, s := range sets {
		lm := lastMod[s.Prefix]
		if !opts.Force && lm != 0 && stored[s.Prefix] == lm {
			res.Skipped++
			continue
		}
		todo = append(todo, job{set: s, lm: lm})
	}

	if err := upsertSets(ctx, db, sets); err != nil {
		return nil, err
	}
	res.Sets = len(sets)

	type outcome struct {
		prefix string
		rows   []IconRow
		lm     int64
		err    error
	}
	results := make(chan outcome, len(todo))
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	for _, j := range todo {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			raw, err := api.Get(ctx, "/collection", map[string]string{"prefix": j.set.Prefix})
			if err != nil {
				results <- outcome{prefix: j.set.Prefix, err: err}
				return
			}
			rows, err := ParseCollection(j.set.Prefix, raw)
			results <- outcome{prefix: j.set.Prefix, rows: rows, lm: j.lm, err: err}
		}(j)
	}
	go func() { wg.Wait(); close(results) }()

	// Writes are serialized here on purpose: the fetches above run in parallel,
	// but SQLite allows a single writer, and concurrent replaceIcons
	// transactions silently lost sets to lock contention.
	done := 0
	var firstWriteErr error
	for r := range results {
		done++
		if opts.OnProgress != nil {
			opts.OnProgress(done, len(todo), r.prefix)
		}
		if r.err != nil {
			res.Failed = append(res.Failed, r.prefix)
			continue
		}
		if err := replaceIcons(ctx, db, r.prefix, r.rows, r.lm); err != nil {
			res.Failed = append(res.Failed, r.prefix)
			if firstWriteErr == nil {
				firstWriteErr = fmt.Errorf("writing %s: %w", r.prefix, err)
			}
			continue
		}
		res.Refreshed = append(res.Refreshed, r.prefix)
		res.Icons += len(r.rows)
	}
	// A partial index is worse than a loud failure: the caller would get empty
	// coverage results for the missing sets with no signal that they are absent.
	if firstWriteErr != nil && len(res.Failed) > len(sets)/10 {
		return res, fmt.Errorf("indexing failed for %d of %d sets: %w", len(res.Failed), len(sets), firstWriteErr)
	}
	sort.Strings(res.Refreshed)
	sort.Strings(res.Failed)
	res.DurationMS = time.Since(started).Milliseconds()
	return res, nil
}

func storedLastModified(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	rows, err := db.QueryContext(ctx, `SELECT prefix, last_modified FROM icon_sets`)
	if err != nil {
		return nil, fmt.Errorf("reading stored last-modified: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var lm sql.NullInt64
		if err := rows.Scan(&p, &lm); err != nil {
			return nil, fmt.Errorf("scanning stored last-modified: %w", err)
		}
		out[p] = lm.Int64
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stored last-modified: %w", err)
	}
	return out, nil
}

func upsertSets(ctx context.Context, db *sql.DB, sets []SetInfo) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sets tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO icon_sets (prefix,name,total,category,author_name,license_title,license_spdx,license_url,palette,height,last_modified,indexed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,0,?)
		ON CONFLICT(prefix) DO UPDATE SET
			name=excluded.name, total=excluded.total, category=excluded.category,
			author_name=excluded.author_name, license_title=excluded.license_title,
			license_spdx=excluded.license_spdx, license_url=excluded.license_url,
			palette=excluded.palette, height=excluded.height, indexed_at=excluded.indexed_at`)
	if err != nil {
		return fmt.Errorf("prepare sets upsert: %w", err)
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, s := range sets {
		palette := 0
		if s.Palette {
			palette = 1
		}
		if _, err := stmt.ExecContext(ctx, s.Prefix, s.Name, s.Total, s.Category, s.Author,
			s.License, s.SPDX, s.URL, palette, s.Height, now); err != nil {
			return fmt.Errorf("upserting set %s: %w", s.Prefix, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sets tx: %w", err)
	}
	return nil
}

func replaceIcons(ctx context.Context, db *sql.DB, prefix string, rows []IconRow, lm int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin icons tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Snapshot the prior name set before replacing it, so `diff` has something
	// to compare against on the next refresh.
	prior, err := namesInTx(ctx, tx, prefix)
	if err != nil {
		return err
	}
	if len(prior) > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO icon_set_snapshots (prefix,taken_at,last_modified,names) VALUES (?,?,?,?)`,
			prefix, time.Now().Unix(), lm, strings.Join(prior, "\n")); err != nil {
			return fmt.Errorf("snapshotting %s: %w", prefix, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM icons WHERE prefix = ?`, prefix); err != nil {
		return fmt.Errorf("clearing icons for %s: %w", prefix, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM icons_fts WHERE prefix = ?`, prefix); err != nil {
		return fmt.Errorf("clearing fts for %s: %w", prefix, err)
	}
	ins, err := tx.PrepareContext(ctx, `INSERT INTO icons (prefix,name,category,hidden,alias_of) VALUES (?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare icons insert: %w", err)
	}
	defer ins.Close()
	fts, err := tx.PrepareContext(ctx, `INSERT INTO icons_fts (prefix,name,category) VALUES (?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare fts insert: %w", err)
	}
	defer fts.Close()
	for _, r := range rows {
		hidden := 0
		if r.Hidden {
			hidden = 1
		}
		if _, err := ins.ExecContext(ctx, r.Prefix, r.Name, r.Category, hidden, r.AliasOf); err != nil {
			return fmt.Errorf("inserting %s:%s: %w", r.Prefix, r.Name, err)
		}
		if !r.Hidden {
			if _, err := fts.ExecContext(ctx, r.Prefix, r.Name, r.Category); err != nil {
				return fmt.Errorf("indexing %s:%s: %w", r.Prefix, r.Name, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE icon_sets SET last_modified = ? WHERE prefix = ?`, lm, prefix); err != nil {
		return fmt.Errorf("recording last-modified for %s: %w", prefix, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit icons tx: %w", err)
	}
	return nil
}

func namesInTx(ctx context.Context, tx *sql.Tx, prefix string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM icons WHERE prefix = ? ORDER BY name`, prefix)
	if err != nil {
		return nil, fmt.Errorf("reading prior names for %s: %w", prefix, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n sql.NullString
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scanning prior name for %s: %w", prefix, err)
		}
		out = append(out, n.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating prior names for %s: %w", prefix, err)
	}
	return out, nil
}
