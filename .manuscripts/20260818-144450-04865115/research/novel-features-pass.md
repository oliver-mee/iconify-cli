## Customer model

**Persona A — the deck builder.** Runs a slide/deck build system that assembles branded HTML→PPTX decks. Every deck needs 10-30 icons in a brand grey `#404041`, all at one size, all from one set.

- **Today (without this CLI):** Has icones.antfu.dev open in one tab, copies an icon name, hand-writes a `curl https://api.iconify.design/lucide/rocket.svg?color=%23404041&width=32 -o assets/rocket.svg`, repeats 20 times, then discovers three of the names silently resolved to something else or came back as a 404 he pasted into the deck as an empty file. Cannot answer: "are all 22 icons in this deck actually from the same set, at the same grid height?"
- **Weekly ritual:** Build or revise a deck; produce a directory of recoloured, uniformly-sized SVGs from a list of icon concepts he wrote in the copy pass.
- **Frustration:** The list-of-concepts → directory-of-correct-SVGs step is 20 manual round-trips with no verification, and it has to be redone from scratch whenever the copy changes.

**Persona B — "The coding agent in a shadcn project."** A Claude/Codex session editing a Next.js app that uses the paid shadcn.io registry. Needs an icon component, not an SVG file.

- **Today (without this CLI):** Cannot browse. Guesses a Lucide name from memory, writes `<Home />`, and either the import resolves or the build breaks. When it does reach for the registry it guesses at `shadcn add @shadcnio/...` slugs. Cannot answer: "does an icon matching this concept exist, and what is the exact registry slug to install it?"
- **Weekly ritual:** Multiple times per session — resolve a described icon into a name that exists, then into an install command that works.
- **Frustration:** The gap between "I want a looping-arrow icon" and "the exact string that installs it" is unbridgeable without a browser, and a wrong guess is a failed build, not a warning.

**Persona C — "The frontend dev inheriting a mixed-icon codebase."** Owns a repo where icons arrived from five sets at five stroke weights over two years.

- **Today (without this CLI):** greps for `@iconify` / `~icons/` imports, eyeballs the prefixes, opens each set's page to compare grid heights, and gives up on migration because they can't tell in advance whether the target set covers all 60 icons already in use.
- **Weekly ritual:** Reviewing PRs that add icons, trying to hold the line on one set.
- **Frustration:** Cannot answer "if we standardise on `lucide`, which of our 60 current icons have no equivalent?" without 60 manual lookups — so the migration never starts and the drift continues.

**Persona D — "The build engineer pinning an icon dependency."** Ships a design system that vendors icon names into a component library.

- **Today (without this CLI):** Icon sets update silently upstream. A name gets deprecated into an alias, or an icon is removed, and it surfaces as a blank square in production. No changelog exists per set.
- **Weekly ritual:** A CI or pre-release check that nothing in the icon manifest has moved.
- **Frustration:** There is no diff. `/last-modified` says a set changed; nothing says *what* changed.

## Candidates (pre-cut)

| # | Name | Command | Description | Persona | Source | Verdict | Long Description |
|---|------|---------|-------------|---------|--------|---------|------------------|
| 1 | Set coverage picker | `iconify set-pick <concept...>` | Given N icon concepts, rank every synced set by how many it covers; show the misses per set | C, A | (c) cross-entity local query | **keep** — pure local FTS aggregation grouped by prefix, no API call | needed (overlaps `swap`, `audit`) |
| 2 | shadcn.io slug bridge | `iconify shadcn <icon...>` | Map `set:name` → `@shadcnio/set-name`, verify the icon exists locally, print/exec the `shadcn add` line | B | (e) user vision | **keep** — deterministic slug map verified across lucide/carbon/akar-icons/bi; `--install` shells to an already-installed CLI, not a network service | needed (overlaps `set-pick`) |
| 3 | Manifest-driven icon kit | `iconify kit <manifest>` | Read a YAML/JSON list of icons + a colour/size profile, write the whole recoloured SVG set to a directory, report aliases and misses | A | (a) persona-driven | **keep** — batches `/{prefix}/{name}.svg` with `color`/`width`/`flip`/`rotate`, idempotent re-run | needed (overlaps `icons get`) |
| 4 | Repo icon audit | `iconify audit <path>` | Scan source files for Iconify icon references; report set spread, grid-height mismatch, palette (colour vs monotone) mixing, dead names, alias renames | C | (a) persona-driven | **keep** — joins filesystem scan against local `collections` + `icons` | needed (overlaps `set-pick`) |
| 5 | Set drift diff | `iconify diff [--set <prefix>...]` | Compare a stored local snapshot against the live set to list icons added, removed, and newly aliased since last sync | D | (c) cross-entity local query | **keep** — `/last-modified` cursor picks the changed sets, `/collection` supplies the new name union, the local store supplies the old one | needed (overlaps `audit`) |
| 6 | Cross-set migration map | `iconify swap <from> <to> --icons <csv>` | For a list of icons in set A, find the same-name or alias-matched equivalent in set B; report covered / renamed / missing | C | (c) cross-entity local query | **keep** — local self-join on the `icons` table across two prefixes | needed (overlaps `set-pick`) |
| 7 | Batch alias resolution | `iconify resolve <icon...>` | Report the canonical name each requested name resolves to | A, B | (b) service pattern | **reframe** — real quirk, but it is one lookup per name against data already in the store; fold the alias column into `kit`, `audit`, and `swap` output rather than shipping a command | none |
| 8 | Licence filter | `iconify sets list --license <spdx>` / `--commercial-safe` | Filter sets by SPDX licence from `/collections` | C | (b) service pattern | **reframe** — a filter flag on an absorbed command, not a feature; also surface a `licence` column in `set-pick` output so licence participates in the *choose one set* decision | none |
| 9 | Offline name search | `iconify find "<desc>"` | FTS5 search over ~294k names with no `limit` floor | B | (a) | **cut before scoring** — `@saastemly/iconify-mcp` already ships offline FTS5 over the same corpus; this is table stakes, absorbed by `search`, not transcendence | none |
| 10 | Zero-result rescue | `iconify suggest "<desc>"` | When a search returns nothing, expand the term via `/keywords` and re-query | B | (b) service pattern | **reframe** — this is fallback behaviour inside `search`, not a sibling command | none |
| 11 | Set profile card | `iconify explain <prefix>` | Print author, licence, total, grid height, palette, categories for one set | C | (b) | **cut** — verbatim render of a single `/collections?prefix=` response | none |
| 12 | Visual similarity | `iconify similar <icon>` | Find visually similar icons across sets by comparing SVG path geometry | A | (b) | **cut** — needs a render/geometry pipeline; unverifiable in dogfood; fails scope creep | none |
| 13 | Corpus stats | `iconify stats` | Totals by category, palette, licence across all 236 sets | — | (c) | **cut** — no persona runs this weekly; it is a curiosity, and `analytics --type collections --group-by category` already covers it | none |
| 14 | Interactive picker | `iconify browse` | TUI grid to arrow-key through search results | A | (a) | **cut** — TUI, persistent process, and it re-implements `icones` in a terminal | none |
| 15 | Brand profile presets | `iconify profile use brand` | Named colour/size profiles applied to every fetch | A | (e) user vision | **reframe** — config, not a command; it is the profile block `kit` reads | none |
| 16 | Deck export | `iconify deck-export <deck.json>` | Emit icons shaped for the slide-build system's asset layout | A | (e) | **cut** — same mechanism as `kit` with one consumer's directory convention baked in; `kit`'s manifest covers it without coupling the CLI to one downstream system | none |

## Survivors and kills

### Survivors

| # | Feature | Command | Score | Buildability | How It Works | Evidence | Long Description |
|---|---------|---------|-------|--------------|-------------|----------|------------------|
| 1 | Set coverage picker | `iconify set-pick "rocket" "shield" "handshake"` | 10/10 | hand-code | Runs one local FTS5 query per concept over the synced `icons` table, groups hits by prefix, and ranks sets by concepts-covered with the misses and each set's licence and grid height listed per row — no API call. | Brief User Vision ("offline search over 294k icons is the headline differentiator"); operator's stated value of picking ONE set and never shipping five sets at five stroke weights; no surveyed tool (icones, iconify-cli, pyconify, both MCP servers) offers coverage-by-set. | Use this command to choose which single icon set to standardise on, given a list of icon concepts you need. Do NOT use it to migrate icons you already use from one set to another; use `iconify swap` instead. Do NOT use it to inspect icons already present in a codebase; use `iconify audit` instead. |
| 2 | shadcn.io registry bridge | `iconify shadcn lucide:home carbon:rocket --install` | 9/10 | hand-code | Verifies each `set:name` against the local `icons` table (resolving aliases), then emits the deterministic `@shadcnio/<set>-<name>` slug and the `shadcn add` line; `--install` execs the already-installed `shadcn` binary. | Slug mapping verified live across lucide, carbon, akar-icons, and bi; brief User Vision names the registry bridge explicitly; no existing tool in the survey bridges Iconify names to a component registry. | Use this command to turn an Iconify icon name into a shadcn.io registry install. Do NOT use it to decide *which* icon or set to use; use `iconify set-pick` first, then pass its output here. |
| 3 | Manifest-driven icon kit | `iconify kit icons.yaml --out assets/ --color '#404041' --width 32` | 9/10 | hand-code | Reads a manifest of icon names plus a colour/size/transform profile and batches `/{prefix}/{name}.svg` with `color`, `width`, `flip`, and `rotate`, writing one file per entry and reporting alias renames and misses as a summary; re-running with an unchanged manifest is a no-op. | Brief User Vision ("recoloured SVG output feeds an existing slide-building workflow") plus Top Workflow 2; surveyed tools download one icon at a time (`bytelab-studio/iconify-cli download`) or generate framework code, none take a manifest with a render profile. | Use this command to produce a whole directory of recoloured, uniformly sized SVGs from a list. Do NOT use it to fetch a single ad-hoc icon; use `iconify icons get` instead. |
| 4 | Repo icon audit | `iconify audit ./src` | 8/10 | hand-code | Greps source files for Iconify references (`@iconify/*`, `~icons/*`, `prefix:name` literals), then joins every hit against the local `icons` and `collections` tables to report set spread, mismatched grid heights, colour-vs-monotone palette mixing, aliased-away names, and names that no longer exist. | Brief's identity thesis that names resolve silently through aliases (`lucide:home` → `house`); `/collections` exposes per-set `height` and `palette`, which nothing in the surveyed tooling uses; operator's stated anti-pattern of five sets at five stroke weights. | Use this command to inspect icons already referenced in a codebase. Do NOT use it to pick a target set for new work; use `iconify set-pick` instead. Do NOT use it to detect upstream changes to a set; use `iconify diff` instead. |
| 5 | Set drift diff | `iconify diff --set lucide --set mdi` | 8/10 | hand-code | Calls `/last-modified` once to find which synced sets advanced, pulls `/collection` only for those, and diffs the fresh name union against the stored snapshot to list added, removed, and newly aliased icons. | Brief Data Layer names `/last-modified` as the incremental-sync cursor; `pyapp-kit/pyconify` exposes `last_modified` as a raw value with no diff semantics, and no surveyed tool stores snapshots to compare against. | Use this command to see what changed upstream in an icon set since your last sync. Do NOT use it to find problems in your own code; use `iconify audit` instead. |
| 6 | Cross-set migration map | `iconify swap mdi lucide --icons home,account,cog,rocket` | 7/10 | hand-code | Self-joins the local `icons` table across two prefixes, matching each source name against the target set's canonical names and aliases, and classifies every icon as covered, renamed, or missing. | Complements `set-pick` with the execution half of the same operator value (one set, consistently); requires the alias table that `/collection` returns and only the local store makes joinable — none of the surveyed tools expose cross-set queries. | Use this command when you have already chosen a target set and need to migrate specific existing icons to it. Do NOT use it to choose the target set in the first place; use `iconify set-pick` instead. |

**Pass 3 force-answers (abbreviated per survivor):**

1. `set-pick` — weekly: yes, every new page or deck starts here. Not a wrapper: no endpoint returns coverage-by-set. Transcendence: local SQLite aggregation. Sibling killed: `explain` (thin `/collections` render). `hand-code`, `// pp:data-source local` (rejects `--data-source live`).
2. `shadcn` — weekly: multiple times per agent session. Not a wrapper: the API knows nothing about shadcn.io. Transcendence: agent-shaped output plus local existence check. Sibling killed: `resolve` (its alias check is subsumed here). `hand-code`, `// pp:data-source local`.
3. `kit` — weekly: yes, once per deck revision. Not a wrapper: `/{prefix}/{name}.svg` is one icon; this is manifest → directory with idempotency and a miss report. Transcendence: service-specific render params driving a batch. Sibling killed: `deck-export` (same mechanism, one consumer's layout hardcoded). `hand-code`, `// pp:data-source auto`.
4. `audit` — weekly: yes, on PR review. Not a wrapper: no endpoint reads a filesystem. Transcendence: cross-source join (repo × local store). Sibling killed: `resolve`. `hand-code`, `// pp:data-source local`.
5. `diff` — weekly: yes, as a CI/pre-release check. Not a wrapper: `/last-modified` returns a timestamp, not a changeset. Transcendence: local snapshot join. Sibling killed: `stats`. `hand-code`, `// pp:data-source auto`.
6. `swap` — weekly: borderline, but it is the recurring PR-review question for Persona C and runs per-migration-batch, so it clears the soft-kill bar. Not a wrapper: cross-prefix join. Transcendence: local SQLite self-join over aliases. Sibling killed: `similar` (the visual version of the same question, unbuildable). `hand-code`, `// pp:data-source local`.

All six read the local store, so each calls the sync hint helpers before returning results — `hintIfUnsynced(cmd, db, "icons")` / `hintIfStale(...)`, using `""` for `audit` and `set-pick` where the scan spans both resources. All joins follow the drain-first pattern: scan the FTS result set into structs, check `rows.Err()`, close, then run per-prefix or alias-resolution follow-ups on the same connection.

### Killed candidates

| Feature | Kill reason | Closest surviving sibling |
|---------|-------------|---------------------------|
| Offline name search (`find`) | `@saastemly/iconify-mcp` already ships offline FTS5 over ~312k icons across 214 sets — this is absorbed table stakes, not transcendence. | absorbed `search` |
| Batch alias resolution (`resolve`) | One store lookup per name with no join or synthesis; the alias column belongs in the commands that already touch names. | `audit` |
| Licence filter as a command | A filter flag on an absorbed `sets list`, not a feature; licence instead appears as a column in `set-pick` output. | `set-pick` |
| Zero-result rescue (`suggest`) | Fallback behaviour inside `search` via `/keywords`, not a sibling command. | absorbed `search` |
| Set profile card (`explain`) | Verbatim render of a single `/collections?prefix=` response — a thin wrapper. | absorbed `sets list` |
| Visual similarity (`similar`) | Requires an SVG geometry/render pipeline and produces output nothing in dogfood can verify. | `swap` |
| Corpus stats (`stats`) | No persona runs it weekly, and generated `analytics --type collections --group-by category` already covers it. | `set-pick` |
| Interactive picker (`browse`) | TUI plus persistent process; re-implements `icones` in a terminal. | absorbed `search` |
| Brand profile presets | Configuration, not a command — it is the profile block `kit` reads from `~/.config/iconify/config.toml`. | `kit` |
| Deck export | Identical mechanism to `kit` with one downstream system's directory convention baked into the CLI. | `kit` |

No `## Reprint verdicts` section: `${PRIOR_RESEARCH_PATH}` is `none`, so this is a first print.
