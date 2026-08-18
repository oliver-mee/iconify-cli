package iconindex

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// SetMeta is the stored metadata for one icon set.
type SetMeta struct {
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Total    int    `json:"total"`
	Category string `json:"category,omitempty"`
	License  string `json:"license,omitempty"`
	SPDX     string `json:"license_spdx,omitempty"`
	Palette  bool   `json:"palette"`
	Height   int    `json:"height,omitempty"`
}

// FTSQuery turns a free-text concept into an FTS5 prefix query. Each word
// becomes a prefix term so "arrow right" also matches "arrow-right-circle";
// FTS5 syntax characters are stripped so a user's punctuation cannot produce
// a malformed query.
func FTSQuery(concept string) string {
	fields := strings.FieldsFunc(strings.ToLower(concept), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	if len(fields) == 0 {
		return ""
	}
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		terms = append(terms, f+"*")
	}
	return strings.Join(terms, " AND ")
}

// Coverage is one set's coverage of a list of concepts.
type Coverage struct {
	SetMeta
	Covered  int               `json:"covered"`
	Total    int               `json:"concepts_total"`
	Matches  map[string]string `json:"matches"`
	Missing  []string          `json:"missing"`
	Complete bool              `json:"complete"`
}

// CoverageBySet answers "which single icon set covers all of these concepts",
// which no API endpoint can answer: it needs one search per concept, grouped
// by set, across the whole corpus.
func CoverageBySet(ctx context.Context, db *sql.DB, concepts []string, limitSets int) ([]Coverage, error) {
	if len(concepts) == 0 {
		return nil, fmt.Errorf("at least one concept is required")
	}
	// concept -> prefix -> best matching icon name
	hits := make(map[string]map[string]string, len(concepts))
	for _, concept := range concepts {
		q := FTSQuery(concept)
		if q == "" {
			continue
		}
		perSet, err := bestPerSet(ctx, db, q)
		if err != nil {
			return nil, err
		}
		hits[concept] = perSet
	}

	prefixes := map[string]bool{}
	for _, perSet := range hits {
		for p := range perSet {
			prefixes[p] = true
		}
	}
	if len(prefixes) == 0 {
		return []Coverage{}, nil
	}
	metas, err := SetsMeta(ctx, db, keys(prefixes))
	if err != nil {
		return nil, err
	}

	out := make([]Coverage, 0, len(metas))
	for _, m := range metas {
		c := Coverage{SetMeta: m, Total: len(concepts), Matches: map[string]string{}, Missing: []string{}}
		for _, concept := range concepts {
			if name, ok := hits[concept][m.Prefix]; ok {
				c.Matches[concept] = name
				c.Covered++
			} else {
				c.Missing = append(c.Missing, concept)
			}
		}
		c.Complete = c.Covered == c.Total
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Covered != out[j].Covered {
			return out[i].Covered > out[j].Covered
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Prefix < out[j].Prefix
	})
	if limitSets > 0 && len(out) > limitSets {
		out = out[:limitSets]
	}
	return out, nil
}

func bestPerSet(ctx context.Context, db *sql.DB, ftsQuery string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT prefix, name FROM icons_fts WHERE icons_fts MATCH ? ORDER BY rank LIMIT 4000`, ftsQuery)
	if err != nil {
		return nil, fmt.Errorf("searching index: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, n sql.NullString
		if err := rows.Scan(&p, &n); err != nil {
			return nil, fmt.Errorf("scanning search hit: %w", err)
		}
		if _, seen := out[p.String]; !seen {
			out[p.String] = n.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search hits: %w", err)
	}
	return out, nil
}

// SetsMeta loads metadata for the given prefixes, or all sets when empty.
func SetsMeta(ctx context.Context, db *sql.DB, prefixes []string) ([]SetMeta, error) {
	q := `SELECT prefix,name,total,category,license_title,license_spdx,palette,height FROM icon_sets`
	var args []any
	if len(prefixes) > 0 {
		// #nosec G202 -- placeholders() emits only "?,?,?"; every prefix value
		// is bound as a query argument below.
		q += ` WHERE prefix IN (` + placeholders(len(prefixes)) + `)`
		for _, p := range prefixes {
			args = append(args, p)
		}
	}
	q += ` ORDER BY prefix`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reading icon sets: %w", err)
	}
	defer rows.Close()
	out := []SetMeta{}
	for rows.Next() {
		var m SetMeta
		var name, category, license, spdx sql.NullString
		var total, palette, height sql.NullInt64
		if err := rows.Scan(&m.Prefix, &name, &total, &category, &license, &spdx, &palette, &height); err != nil {
			return nil, fmt.Errorf("scanning icon set: %w", err)
		}
		m.Name, m.Category, m.License, m.SPDX = name.String, category.String, license.String, spdx.String
		m.Total, m.Height, m.Palette = int(total.Int64), int(height.Int64), palette.Int64 == 1
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating icon sets: %w", err)
	}
	return out, nil
}

// Resolution is the outcome of resolving one prefix:name against the index.
type Resolution struct {
	Query     string `json:"query"`
	Prefix    string `json:"prefix"`
	Name      string `json:"name"`
	Canonical string `json:"canonical"`
	Exists    bool   `json:"exists"`
	Aliased   bool   `json:"aliased"`
	Hidden    bool   `json:"hidden"`
}

// Resolve looks one icon up, following the alias chain. Iconify renames
// silently (lucide:home serves house), so surfacing the canonical name is what
// keeps a caller from believing they installed a name that no longer exists.
func Resolve(ctx context.Context, db *sql.DB, prefix, name string) (Resolution, error) {
	r := Resolution{Query: prefix + ":" + name, Prefix: prefix, Name: name, Canonical: name}
	var aliasOf sql.NullString
	var hidden sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT alias_of, hidden FROM icons WHERE prefix = ? AND name = ?`, prefix, name).Scan(&aliasOf, &hidden)
	if err == sql.ErrNoRows {
		return r, nil
	}
	if err != nil {
		return r, fmt.Errorf("resolving %s:%s: %w", prefix, name, err)
	}
	r.Exists = true
	r.Hidden = hidden.Int64 == 1
	if aliasOf.String != "" {
		r.Aliased = true
		r.Canonical = aliasOf.String
	}
	return r, nil
}

// SwapRow classifies one icon's fate when moving between sets.
type SwapRow struct {
	Name   string `json:"name"`
	Status string `json:"status"` // covered | renamed | missing
	Target string `json:"target,omitempty"`
	Note   string `json:"note,omitempty"`
}

// CrossSet maps names from one set onto another, matching canonical names
// first and then the target's aliases. This is a self-join over the local
// mirror; the API exposes no cross-set query.
func CrossSet(ctx context.Context, db *sql.DB, from, to string, names []string) ([]SwapRow, error) {
	targetNames, targetAliases, err := setIndex(ctx, db, to)
	if err != nil {
		return nil, err
	}
	out := make([]SwapRow, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		switch {
		case targetNames[n]:
			out = append(out, SwapRow{Name: n, Status: "covered", Target: to + ":" + n})
		case targetAliases[n] != "":
			out = append(out, SwapRow{Name: n, Status: "renamed", Target: to + ":" + targetAliases[n],
				Note: "resolves through an alias in " + to})
		default:
			out = append(out, SwapRow{Name: n, Status: "missing",
				Note: "no icon of this name in " + to})
		}
	}
	return out, nil
}

func setIndex(ctx context.Context, db *sql.DB, prefix string) (map[string]bool, map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, alias_of FROM icons WHERE prefix = ? AND hidden = 0`, prefix)
	if err != nil {
		return nil, nil, fmt.Errorf("reading set %s: %w", prefix, err)
	}
	defer rows.Close()
	names := map[string]bool{}
	aliases := map[string]string{}
	for rows.Next() {
		var n, a sql.NullString
		if err := rows.Scan(&n, &a); err != nil {
			return nil, nil, fmt.Errorf("scanning set %s: %w", prefix, err)
		}
		if a.String != "" {
			aliases[n.String] = a.String
			continue
		}
		names[n.String] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating set %s: %w", prefix, err)
	}
	return names, aliases, nil
}

// SetNames returns the visible icon names for a prefix.
func SetNames(ctx context.Context, db *sql.DB, prefix string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM icons WHERE prefix = ? AND hidden = 0 ORDER BY name`, prefix)
	if err != nil {
		return nil, fmt.Errorf("reading names for %s: %w", prefix, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n sql.NullString
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scanning name for %s: %w", prefix, err)
		}
		out = append(out, n.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating names for %s: %w", prefix, err)
	}
	return out, nil
}

// Snapshot is a stored point-in-time name list for a set.
type Snapshot struct {
	Prefix  string
	TakenAt int64
	Names   []string
}

// LatestSnapshot returns the most recent stored snapshot for a prefix.
func LatestSnapshot(ctx context.Context, db *sql.DB, prefix string) (*Snapshot, error) {
	var takenAt sql.NullInt64
	var names sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT taken_at, names FROM icon_set_snapshots WHERE prefix = ? ORDER BY taken_at DESC LIMIT 1`,
		prefix).Scan(&takenAt, &names)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading snapshot for %s: %w", prefix, err)
	}
	s := &Snapshot{Prefix: prefix, TakenAt: takenAt.Int64}
	if names.String != "" {
		s.Names = strings.Split(names.String, "\n")
	}
	return s, nil
}

// DiffNames reports what changed between two name lists.
func DiffNames(before, after []string) (added, removed []string) {
	b := make(map[string]bool, len(before))
	for _, n := range before {
		b[n] = true
	}
	a := make(map[string]bool, len(after))
	for _, n := range after {
		a[n] = true
	}
	added, removed = []string{}, []string{}
	for n := range a {
		if !b[n] {
			added = append(added, n)
		}
	}
	for n := range b {
		if !a[n] {
			removed = append(removed, n)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// UnindexedPrefixes returns, in input order, the prefixes that have no visible
// icon rows in the local index.
//
// A command that scopes its answer to explicit set prefixes must report these
// rather than treating an absent set as an empty one. CrossSet, for instance,
// only builds an index for its target set, so an unindexed source set would
// otherwise read as "every icon migrates cleanly" instead of "I have no idea
// what is in that set".
func UnindexedPrefixes(ctx context.Context, db *sql.DB, prefixes []string) ([]string, error) {
	var missing []string
	seen := map[string]bool{}
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		var n int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM icons WHERE prefix = ? AND hidden = 0`, p).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("checking index coverage for %s: %w", p, err)
		}
		if n == 0 {
			missing = append(missing, p)
		}
	}
	return missing, nil
}
