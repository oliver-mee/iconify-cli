# Iconify CLI Absorb Manifest

## Tools surveyed

| Tool | Kind | Signal | What it proves |
|---|---|---|---|
| `pyapp-kit/pyconify` | Python library | 7 stars, active | Broadest existing API-surface coverage |
| `@saastemly/iconify-mcp` | MCP server (npm) | 9 weekly downloads, no public repo | **Offline FTS5 search over ~312k icons already exists** |
| `imjac0b/iconify-mcp-server` | MCP server | 14 stars, stale | 4-tool MCP shape |
| `bytelab-studio/iconify-cli` | TS CLI | 4 stars | Only general-purpose CLI; config/search/download/describe/sets |
| `ksckaan1/templ-iconify` | Go codegen | 10 stars | templ components |
| `davidB/dioxus-iconify`, `nklsw/cotton-iconify` | codegen | 11 / 2 stars | Framework-coupled |
| `antfu-collective/icones` | GUI | 7,438 stars | The discovery UX everyone actually uses |
| `@iconify/utils`, `@iconify/tools`, `@iconify/json` | Official libs | — | No official CLI exists |

## Absorbed (match or beat everything that exists)

| # | Feature | Best Source | Our Implementation | Added Value |
|---|---------|-----------|-------------------|-------------|
| 1 | List all icon sets | imjac0b `get_all_icon_sets`, pyconify `collections` | (generated endpoint) sets list | `--json`/`--csv`/`--select`, partial-prefix family filter, offline after sync |
| 2 | Inspect one icon set | imjac0b `get_icon_set`, bytelab `describe collection` | (generated endpoint) sets get | Surfaces categories, aliases, and hidden icons rather than a flat count |
| 3 | Search icons | pyconify `search`, bytelab `search` | (generated endpoint) icons search | Local index removes the API's silent 32-result floor |
| 4 | Fetch icon as SVG | pyconify `svg`, imjac0b `get_icon` | (generated endpoint) icons get | Full render surface: color, width, height, flip, rotate, box |
| 5 | Save SVG to a file | pyconify `svg_path`, bytelab `download` | (behavior in iconify icons get) `--output` writes the SVG to a path, stdout otherwise | No template config required to get one file |
| 6 | Bulk icon data | pyconify `icon_data` | (generated endpoint) icons data | One call for many icons, pipeable |
| 7 | CSS sprite generation | pyconify `css` | (generated endpoint) icons css | Streams to stdout for build pipelines |
| 8 | Keyword suggestions | pyconify `keywords` | (generated endpoint) keywords list | Feeds search-term expansion |
| 9 | Last-modified lookup | pyconify `last_modified` | (generated endpoint) meta last-modified | Doubles as the incremental sync cursor |
| 10 | API version | pyconify `iconify_version` | (generated endpoint) meta version | Also the doctor health probe |
| 11 | Response caching | pyconify `clear_api_cache` / `set_api_cache_maxsize` | (behavior in iconify sync) persistent SQLite store | Survives process exit; pyconify's cache is per-session only |
| 12 | Offline icon search index | `@saastemly/iconify-mcp` (FTS5, ~312k icons) | (behavior in iconify search) FTS5 over the synced local store | Same capability, delivered as a single Go binary with no Node runtime, and reachable from a CLI rather than MCP only |
| 13 | Template-based output | bytelab `--template` | (behavior in iconify icons get) `--json` plus `--select` | Composes with jq instead of a bespoke template language |
| 14 | Config file | bytelab `config init` | (generated) config | Standard printing-press config path and doctor |
| 15 | MCP server surface | imjac0b, saastemly | (generated) iconify-pp-mcp | Mirrors the whole Cobra tree automatically, so every command is an agent tool |

**Honest note on row 12.** Offline search was drafted as a differentiator and demoted to absorbed once `@saastemly/iconify-mcp` was found doing exactly that. The remaining edge is packaging (one static Go binary, CLI + MCP from one build) and reach, not the capability itself.

## Transcendence (only possible with our approach)

All six are `hand-code`: the generator emits none of them.

| # | Feature | Command | Buildability | Why Only We Can Do This | Long Description |
|---|---------|---------|--------------|------------------------|------------------|
| 1 | Set coverage picker | set-pick | hand-code | One local FTS5 query per concept, grouped by prefix and ranked by concepts-covered, with licence and grid height per row. No endpoint returns coverage-by-set. | Use this command to choose which single icon set to standardise on, given a list of icon concepts you need. Do NOT use it to migrate icons you already use; use `swap`. Do NOT use it to inspect icons already in a codebase; use `audit`. |
| 2 | shadcn.io registry bridge | shadcn | hand-code | Verifies each `set:name` against the local store, resolves aliases, emits the deterministic `@shadcnio/<set>-<name>` slug. The API knows nothing about shadcn.io. | Use this command to turn an Iconify icon name into a shadcn.io registry install. Do NOT use it to decide which icon or set to use; use `set-pick` first. |
| 3 | Manifest-driven icon kit | kit | hand-code | Manifest plus a render profile batched over `/{prefix}/{name}.svg`, one file per entry, idempotent, with alias renames and misses reported. Surveyed tools fetch one icon at a time. | Use this command to produce a directory of recoloured, uniformly sized SVGs from a list. Do NOT use it for a single ad-hoc icon; use `icons get`. |
| 4 | Repo icon audit | audit | hand-code | Greps source for Iconify references, joins against local `icons` and `collections` to report set spread, mismatched grid heights, palette mixing, aliased-away and dead names. No endpoint reads a filesystem. | Use this command to inspect icons already referenced in a codebase. Do NOT use it to pick a target set; use `set-pick`. Do NOT use it to detect upstream changes; use `diff`. |
| 5 | Set drift diff | diff | hand-code | `/last-modified` to find advanced sets, `/collection` only for those, diffed against the stored snapshot. `last_modified` elsewhere is a raw timestamp with no changeset semantics. | Use this command to see what changed upstream in an icon set since your last sync. Do NOT use it to find problems in your own code; use `audit`. |
| 6 | Cross-set migration map | swap | hand-code | Self-joins the local `icons` table across two prefixes against canonical names and aliases, classifying each icon covered / renamed / missing. | Use this command when you have chosen a target set and need to migrate specific existing icons to it. Do NOT use it to choose the target set; use `set-pick`. |

## Killed candidates (recorded so they are not re-litigated)

| Feature | Kill reason |
|---|---|
| Offline name search as a novel feature | `@saastemly/iconify-mcp` already ships offline FTS5 over ~312k icons; absorbed, not novel |
| Batch alias resolution (`resolve`) | One lookup per name with no join; the alias column belongs in the commands that already touch names |
| Licence filter as its own command | A flag on absorbed `sets list`; licence appears as a column in `set-pick` |
| Zero-result rescue (`suggest`) | Fallback behaviour inside `search` via `/keywords` |
| Set profile card (`explain`) | Verbatim render of one `/collections?prefix=` response |
| Visual similarity (`similar`) | Needs an SVG geometry pipeline; output is unverifiable in dogfood |
| Corpus stats (`stats`) | No persona runs it weekly; generated analytics already covers it |
| Interactive picker (`browse`) | TUI plus persistent process; re-implements `icones` in a terminal |
| Brand presets | Configuration, not a command; it is the profile block `kit` reads from config |
| Deck export | Same mechanism as `kit` with one downstream layout hardcoded |
