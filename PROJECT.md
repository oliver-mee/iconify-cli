# iconify-cli

A Go CLI and MCP server for the [Iconify API](https://iconify.design): 236 icon
sets, ~376,000 icons, no authentication required.

## What it is

Generated with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press),
then extended by hand with a local SQLite mirror of the icon corpus and six
commands that mirror cannot be answered by the API alone.

Two binaries ship from one build: `iconify-pp-cli` and `iconify-pp-mcp`.

## Why it exists

Every existing Iconify tool is either a framework-specific code generator or a
thin downloader, and all of them hit the network for every lookup. Nothing let
an agent ask corpus-shaped questions: which single icon set covers everything
this page needs, what breaks if we migrate from mdi to lucide, which icons in
this repo silently resolved through an alias.

## The commands that are not just endpoint wrappers

| Command | Question it answers |
|---|---|
| `index` | Builds the local mirror. Run once; ~40s for all 236 sets. |
| `set-pick` | Which single set covers all these icon concepts? |
| `swap` | What breaks if we migrate these icons from set A to set B? |
| `audit` | What icons does this codebase reference, and are any broken? |
| `diff` | What changed upstream in this set since our last index? |
| `shadcn` | What is the shadcn.io registry slug for this Iconify icon? |
| `kit` | Render a manifest of icons as recoloured, uniformly sized SVGs. |

## Layout

- `internal/iconindex/` — hand-authored: corpus parse, build, and query layers
- `internal/store/iconify_migrations.go` — hand-authored corpus schema
- `internal/cli/{set_pick,shadcn,swap,diff,kit,audit,iconify_index,icons_raw}.go` — hand-authored commands
- everything else is generated; see `CLAUDE.md` for the do-not-edit boundary

Regeneration preserves the hand-authored files via the press's AST-aware merge.

## Docs

`README.md` for install and usage, `SKILL.md` for the agent-facing skill,
`AGENTS.md` for agent conventions.
