# Iconify CLI Brief

## API Identity
- Domain: `https://api.iconify.design` (Iconify API v3.2.0). Icon search and delivery.
- Users: frontend developers and coding agents who need an icon by description, not by name.
- Data profile: 236 icon sets, ~294,000 icons. Names and metadata are small and highly cacheable; SVG bodies are fetched on demand.
- Auth: **none**. No key, no cookie, no rate-limit gate observed. Every endpoint answered 200 unauthenticated.

## Reachability Risk
- None. All nine documented endpoints probed live and returned 200 with the documented content types.
- `/providers` is documented but returns 404 on the public instance; treat as absent, not broken.

## Top Workflows
1. Find an icon by describing it ("arrow that loops back") and get an installable name.
2. Pull one icon as an SVG, recoloured and resized, straight to a file or stdout.
3. Pull a batch of icons for a build step, as one JSON payload or one CSS sprite.
4. Browse what exists: which sets, how big, what licence, what is in a set.
5. Turn an Iconify name into a shadcn.io registry install.

## Table Stakes
- Search with set narrowing, result limits, and pagination.
- Single-icon fetch with the full render parameter set.
- Set listing and set inspection, including aliases and categories.
- Machine-readable output on every command.

## Data Layer
- Primary entities: `collections` (236 rows: prefix, name, total, author, licence, category, palette) and `icons` (~294k rows: prefix, name, aliases, category, hidden).
- Sync cursor: **`/last-modified`**. It returns a per-prefix `lastModified` epoch in one call, which is exactly an incremental-sync cursor. Sync re-pulls only sets whose timestamp advanced.
- FTS/search: FTS5 over icon name plus set name plus category. This is the offline path over ~294k names.

## Endpoint Contract
| endpoint | required | optional |
|---|---|---|
| `/search` | `query` | `limit` (min 32, default 64, max 999), `start`, `prefix`, `prefixes` (partial ok, `mdi-`), `category` |
| `/collections` | — | `prefix`, `prefixes` |
| `/collection` | `prefix` | `info`, `chars` |
| `/{prefix}.json` | `icons` | — |
| `/{prefix}.css` | `icons` | — |
| `/{prefix}/{name}.svg` | — | `color`, `width`, `height`, `flip`, `rotate`, `download`, `box` |
| `/keywords` | one of `prefix` / `keyword` | — |
| `/last-modified` | — | `prefix`, `prefixes` |
| `/version` | — | — |

Documented quirks worth encoding:
- `limit` below 32 is silently clamped, which is why a caller asking for 6 results gets ~28. Local search removes the floor entirely.
- Icon names resolve through aliases: `lucide:home` returns `house`. Surfacing the resolution avoids a confusing silent rename.
- `/collection` splits names across `uncategorized`, `categories`, `hidden`, and `aliases`; a full name list is the union minus `hidden`.

## Codebase Intelligence
- `iconify/api` (official server source) is the de facto spec; no OpenAPI document is published.
- `pyapp-kit/pyconify` has the broadest existing API-surface coverage of any wrapper and is actively maintained.

## User Vision
- Whole API surface, not a subset.
- Emphasis: core search/get/collections, bulk and CSS export, a shadcn.io registry bridge, save-to-file and keywords.
- Offline search over ~294k icons is the headline differentiator.
- Recoloured SVG output feeds an existing slide-building workflow.
- Ships as `oliver-mee/iconify-cli`, binary `iconify`. Not published upstream.

## Product Thesis
- Name: `iconify`
- Why it should exist: every existing tool is either a framework-specific code generator or a thin downloader, and all of them go over the network for every lookup. Nothing lets an agent search 294k icons offline, and nothing bridges an Iconify name to a shadcn.io registry install. The API is free and unauthenticated, so the only thing standing between an agent and the whole icon corpus is a local index.

## Build Priorities
1. Data layer: sync `collections` + icon names into SQLite with FTS5, cursored on `/last-modified`.
2. Absorbed surface: search, get/download, collections, collection, bulk JSON, CSS, keywords, version.
3. Transcendence: offline search, shadcn.io slug bridge, alias resolution, set diffing, licence-aware filtering.
