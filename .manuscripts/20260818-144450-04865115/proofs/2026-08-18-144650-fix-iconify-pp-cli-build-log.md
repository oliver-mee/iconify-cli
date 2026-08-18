Manifest transcendence rows: 6 planned, 6 built. Phase 3 will not pass until all 6 ship.

# Build log

## Priority 0: data layer (hand-built)
The generated sync path cannot represent the corpus: /collections returns one
object keyed by prefix, and icon names exist only inside per-prefix /collection
responses. Built instead:

- `internal/store/iconify_migrations.go` — icon_sets, icons, icons_fts (FTS5),
  icon_set_snapshots, with lazy schema init.
- `internal/iconindex/` — parse, build, and query layers over that schema.
- `internal/cli/iconify_index.go` — `index` command that fans out across all
  236 sets, cursored on /last-modified.

Result: 236 sets, 376,048 icons, 261k+ FTS rows, ~36s cold build.

### Bug found and fixed during Priority 0
The first build stored **52 of 236 sets with zero icons** (22% silent loss).
Two causes, both fixed:
1. Icon writes ran concurrently; SQLite permits one writer, so parallel
   transactions lost sets to lock contention. Fetches still run in parallel,
   writes are now serialized. The serialized version is also faster (36s vs
   60s) because the lock thrashing cost more than the parallelism gained.
2. `upsertSets` wrote the last_modified cursor before icons landed, so a failed
   set would be skipped on every later run and the gap would never heal. The
   cursor is now written only after a set's icons commit.
A partial index above 10% failure now returns an error rather than reporting
success, because empty coverage results are indistinguishable from real ones.

## Priority 1: absorbed (generated)
All 9 endpoint commands emitted by the generator across icons, sets, keywords,
and meta resources, plus the framework surface (sync, search, sql, export,
doctor, profile, workflow, learn loop).

## Priority 2: transcendence (hand-built, 6/6)
| Command | State | Verified against |
|---|---|---|
| set-pick | built | 3 concepts across the live index, coverage + licence + height |
| shadcn | built | lucide:home -> @shadcnio/lucide-house via alias resolution |
| kit | built | manifest -> 2 SVGs written at #404041 / 32px, misses reported |
| audit | built | demo repo: 3-set spread, mixed heights, palette mix, dead name, alias |
| diff | built | mutated index detects both added and removed names |
| swap | built | mdi -> lucide: covered / renamed / missing all exercised |

## Notes
- `index` was added as a seventh novel command; it is the data layer the other
  six depend on and is recorded in research.json.
- Crowd-sniff could not run: `crowd-sniff` fails with `downloads API returned
  status 400` for both `api.iconify.design` and `iconify`. Machine issue, filed
  as a retro candidate.
- The generated `sync` still reports a warning for the `keywords` resource,
  which requires a parameter and is not listable. Cosmetic; endpoint commands
  are unaffected.
