// Iconify-specific store schema. Hand-authored: the generated migration slice
// in store.go covers the generic resources table, which cannot represent the
// icon corpus. /collections returns one object keyed by prefix rather than a
// list, and icon names only exist inside per-prefix /collection responses, so
// the corpus needs its own tables plus an FTS index to be queryable offline.
//
// Lazily invoked by the commands that read the index, so a CLI that never
// indexes never pays for these tables.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

var iconifyInitOnce sync.Map // dbPath -> *sync.Once

const iconifySchema = `
CREATE TABLE IF NOT EXISTS icon_sets (
	prefix         TEXT PRIMARY KEY,
	name           TEXT NOT NULL DEFAULT '',
	total          INTEGER NOT NULL DEFAULT 0,
	category       TEXT NOT NULL DEFAULT '',
	author_name    TEXT NOT NULL DEFAULT '',
	license_title  TEXT NOT NULL DEFAULT '',
	license_spdx   TEXT NOT NULL DEFAULT '',
	license_url    TEXT NOT NULL DEFAULT '',
	palette        INTEGER NOT NULL DEFAULT 0,
	height         INTEGER NOT NULL DEFAULT 0,
	last_modified  INTEGER NOT NULL DEFAULT 0,
	indexed_at     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS icons (
	prefix   TEXT NOT NULL,
	name     TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	hidden   INTEGER NOT NULL DEFAULT 0,
	alias_of TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (prefix, name)
);

CREATE INDEX IF NOT EXISTS icons_name_idx ON icons(name);
CREATE INDEX IF NOT EXISTS icons_alias_idx ON icons(alias_of) WHERE alias_of <> '';

CREATE VIRTUAL TABLE IF NOT EXISTS icons_fts USING fts5(
	prefix UNINDEXED,
	name,
	category,
	tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS icon_set_snapshots (
	prefix        TEXT NOT NULL,
	taken_at      INTEGER NOT NULL,
	last_modified INTEGER NOT NULL DEFAULT 0,
	names         TEXT NOT NULL,
	PRIMARY KEY (prefix, taken_at)
);
`

// EnsureIconifySchema creates the Iconify corpus tables if they are absent.
// Safe to call repeatedly; the work happens once per database path per process.
func (s *Store) EnsureIconifySchema(ctx context.Context) error {
	v, _ := iconifyInitOnce.LoadOrStore(s.Path(), &sync.Once{})
	once := v.(*sync.Once)
	var err error
	once.Do(func() {
		if _, execErr := s.DB().ExecContext(ctx, iconifySchema); execErr != nil {
			err = fmt.Errorf("creating iconify schema: %w", execErr)
			iconifyInitOnce.Delete(s.Path())
		}
	})
	if err != nil {
		return err
	}
	// Cheap verification so a caller that raced a failed init still errors.
	var name string
	row := s.DB().QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='icons'`)
	if scanErr := row.Scan(&name); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return fmt.Errorf("iconify schema missing after init")
		}
		return fmt.Errorf("verifying iconify schema: %w", scanErr)
	}
	return nil
}

// IconifyIndexed reports how many icon sets and icons are present locally.
func (s *Store) IconifyIndexed(ctx context.Context) (sets int, icons int, err error) {
	if err = s.EnsureIconifySchema(ctx); err != nil {
		return 0, 0, err
	}
	if err = s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM icon_sets`).Scan(&sets); err != nil {
		return 0, 0, fmt.Errorf("counting icon sets: %w", err)
	}
	if err = s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM icons WHERE hidden = 0`).Scan(&icons); err != nil {
		return 0, 0, fmt.Errorf("counting icons: %w", err)
	}
	return sets, icons, nil
}
