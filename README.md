# Iconify CLI

**Every Iconify icon from one Go binary, plus a local index that answers set-coverage, migration, and audit questions no icon tool answers today.**

Search 236 icon sets and roughly 294,000 icons, fetch any icon as a recoloured SVG, and generate CSS or bulk JSON for a build step. The local SQLite mirror turns the corpus into something you can ask real questions of: which single set covers every icon this page needs, what breaks if you migrate from mdi to lucide, and which icons in this repo silently resolved through an alias.

## Install

> **Not in the public Printing Press library yet.** The library's publish gate
> counts a command that can only be tested with `--dry-run` as unverified, which
> blocks any command whose job is writing files (here, `kit`). Tracked upstream in
> [cli-printing-press#4046](https://github.com/mvanhorn/cli-printing-press/issues/4046).
> Until that lands, use **Install from this repository** immediately below. The
> `@mvanhorn/printing-press-library` commands further down will start working once
> it is merged there.

### Install from this repository

Installs the `iconify` and `iconify-pp-mcp` binaries. Requires Go 1.26.6 or newer.

```bash
go install github.com/oliver-mee/iconify-cli/cmd/iconify@latest
go install github.com/oliver-mee/iconify-cli/cmd/iconify-pp-mcp@latest
```

Or build from a clone:

```bash
git clone https://github.com/oliver-mee/iconify-cli.git
cd iconify-cli
make build          # or: go build ./cmd/iconify-pp-cli
```

Then build the local index once, so the corpus commands work offline:

```bash
iconify index          # ~40s for all 236 sets
```

### Install the agent skill

`SKILL.md` in this repository is the agent-facing skill. Install it into any
agent supported by the [`skills`](https://github.com/vercel-labs/skills) CLI:

```bash
npx -y skills@latest add oliver-mee/iconify-cli -g -a claude-code
```

Or install it by hand for Claude Code, by symlinking this repo's `SKILL.md`
into a skill directory named for the skill:

```bash
mkdir -p ~/.claude/skills/pp-iconify
ln -s "$(pwd)/SKILL.md" ~/.claude/skills/pp-iconify/SKILL.md
```

The skill assumes `iconify` is on `PATH`; install the binary first.

### Install the MCP server

`iconify-pp-mcp` mirrors the whole CLI command tree as MCP tools.

```bash
claude mcp add --transport stdio --scope user iconify iconify-pp-mcp
```

For Claude Desktop, a prebuilt `.mcpb` bundle is produced by `make build` under
`build/`; open it to install.

### Install from the Printing Press library

The recommended path installs both the `iconify` binary and the `pp-iconify` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install iconify
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install iconify --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install iconify --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install iconify --agent claude-code
npx -y @mvanhorn/printing-press-library install iconify --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.6 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/iconify/cmd/iconify-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/iconify-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install iconify --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-iconify --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-iconify --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install iconify --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/iconify-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/iconify/cmd/iconify-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "iconify": {
      "command": "iconify-pp-mcp"
    }
  }
}
```

</details>

## Quick Start

```bash
# confirm the API is reachable; no credentials are needed
iconify doctor --dry-run

# mirror all 236 icon sets locally so search and analysis work offline
iconify index

# find an icon by describing it
iconify icons search --query "arrow right" --prefix lucide

# fetch it as an SVG at your brand colour
iconify icons get lucide arrow-right --color '#404041' --width 32

# pick one set that covers everything a page needs
iconify set-pick rocket shield handshake

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`index`** — Mirror icon sets and their icon names into local SQLite for offline search and analysis; --only scopes it to specific sets.

  _Run this once before set-pick, swap, audit, or shadcn; it is what makes them answerable without the network._

  ```bash
  iconify index --only lucide
  ```

### Choose and stay consistent
- **`set-pick`** — Given the icon concepts you need, rank icon sets by how many of them each set actually covers.

  _Reach for this before writing any UI that needs several icons, so every icon comes from one set at one stroke weight._

  ```bash
  iconify set-pick "rocket" "shield" "handshake" --agent
  ```
- **`audit`** — Scan a codebase for Iconify references and report set spread, grid-height mismatches, palette mixing, and dead names.

  _Use this on review to catch icons drifting across sets, or names that silently resolved through an alias._

  ```bash
  iconify audit ./src --agent
  ```
- **`swap`** — Map a list of icons from one set to another, classifying each as covered, renamed, or missing.

  _Use this when standardising an existing codebase onto one icon set, to see the cost before committing._

  ```bash
  iconify swap mdi lucide --icons home,account,cog,rocket --agent
  ```

### Bridges to other tooling
- **`shadcn`** — Turn Iconify icon names into shadcn.io registry slugs and optionally run the install.

  _Use this when the project installs components from shadcn.io and you need the icon as a registry item rather than a raw SVG._

  ```bash
  iconify shadcn lucide:home carbon:rocket --agent
  ```

### Batch rendering
- **`kit`** — Render a whole directory of recoloured, uniformly sized SVGs from a manifest of icon names.

  _Use this when a deck, brand kit, or design system needs many icons at one colour and size rather than one ad-hoc icon._

  ```bash
  iconify kit icons.yaml --out assets/ --color '#404041' --width 32
  ```

### Track upstream change
- **`diff`** — Show which icons an upstream set added, removed, or aliased since your last sync.

  _Use this before upgrading an icon dependency, to see whether a name you rely on disappeared._

  ```bash
  iconify diff --set lucide --agent
  ```

## Recipes

### Find an icon when you only know what it looks like

```bash
iconify icons search --query "arrow that loops back" --limit 32 --agent --select icons
```

Search is fuzzy over names and set metadata, so describing the shape works better than guessing the exact name.

### Render a brand-coloured icon straight to a file

```bash
iconify kit icons.yaml --out assets/ --color '#404041' --width 32
```

Reads a manifest of icon names and writes one recoloured, uniformly sized SVG per entry, reporting any that were renamed upstream.

### Choose one icon set for a whole page

```bash
iconify set-pick rocket shield handshake clock --agent
```

Ranks sets by how many of the listed concepts each covers, with licence and grid height, so you can commit to one set.

### Cost out a migration before doing it

```bash
iconify swap mdi lucide --icons home,account,cog,rocket --agent
```

Classifies every icon as covered, renamed, or missing so you see the gaps before touching code.

### Audit a repo's icon usage on review

```bash
iconify audit ./src --agent --select findings,set_spread
```

Joins the repo's icon references against the local index to surface set sprawl and names that silently resolved through an alias.

## Usage

Run `iconify --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ICONIFY_CONFIG_DIR`, `ICONIFY_DATA_DIR`, `ICONIFY_STATE_DIR`, or `ICONIFY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ICONIFY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ICONIFY_HOME=/srv/iconify
iconify doctor
```

Under `ICONIFY_HOME=/srv/iconify`, the four dirs resolve to `/srv/iconify/config`, `/srv/iconify/data`, `/srv/iconify/state`, and `/srv/iconify/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "iconify": {
      "command": "iconify-pp-mcp",
      "env": {
        "ICONIFY_HOME": "/srv/iconify"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ICONIFY_DATA_DIR` overrides an explicit `--home` for that kind. Use `ICONIFY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ICONIFY_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `iconify doctor --fail-on warn` to check path warnings in automation.

## Commands

### icons

Search and fetch individual icons

- **`iconify icons css`** - Generate a CSS mask sprite for a list of icons from one set
- **`iconify icons data`** - Fetch many icons from one set as a single IconifyJSON payload
- **`iconify icons get`** - Fetch one icon as SVG, optionally recoloured and resized
- **`iconify icons search`** - Search icons by keyword across every icon set

### keywords

Expand partial keywords into search suggestions

- **`iconify keywords`** - Suggest search keywords that start with or end with a fragment

### meta

API metadata and cache invalidation

- **`iconify meta last-modified`** - Get the last modification time per icon set, for cache invalidation and incremental sync
- **`iconify meta version`** - Report the Iconify API version and serving region

### sets

Browse and inspect icon sets

- **`iconify sets get`** - List every icon name in one set, with categories, aliases, and hidden icons
- **`iconify sets list`** - List every icon set with its size, author, licence, and category


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`iconify recall <query>`** - Look up cached resources for a query before running discovery
- **`iconify teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`iconify learnings list`** - Inspect taught rows
- **`iconify learnings forget <query>`** - Undo a teach
- **`iconify learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`iconify learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`iconify teach-pattern`** - Install a query/resource template up front
- **`iconify teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ICONIFY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `iconify` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
iconify icons get mock-value mock-value

# JSON for scripting and agents
iconify icons get mock-value mock-value --json
# Filter to specific fields by name
iconify icons get mock-value mock-value --json --select <field>[,<field>...]

# Dry run — show the request without sending
iconify icons get mock-value mock-value --dry-run

# Agent mode — JSON + compact + no prompts in one flag
iconify icons get mock-value mock-value --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
iconify doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `iconify doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/iconify/config.toml`; `--home`, `ICONIFY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **search returns far more results than the limit you asked for** — The API silently clamps limit below 32. Run 'index' once and the analysis commands resolve locally, where the floor does not apply.
- **an icon name comes back under a different name** — Iconify resolves names through aliases, so lucide:home serves house. Run 'audit' to list every reference in a repo that resolved this way.
- **set-pick, audit, swap, or diff report an empty index** — They read the local mirror. Run 'index' first; it takes about 40 seconds for all 236 sets. If only some sets are missing, 'swap' withholds its verdict and names them under 'unindexed' — run 'index --only <prefix>' for those rather than trusting a partial answer.
- **a set you rely on lost an icon after an upgrade** — Run 'diff --set <prefix>' to list what upstream added or removed since your last index.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**icones**](https://github.com/antfu-collective/icones) — TypeScript (7438 stars)
- [**iconify-mcp-server**](https://github.com/imjac0b/iconify-mcp-server) — TypeScript (14 stars)
- [**dioxus-iconify**](https://github.com/davidB/dioxus-iconify) — Rust (11 stars)
- [**templ-iconify**](https://github.com/ksckaan1/templ-iconify) — Go (10 stars)
- [**pyconify**](https://github.com/pyapp-kit/pyconify) — Python (7 stars)
- [**iconify-cli**](https://github.com/bytelab-studio/iconify-cli) — TypeScript (4 stars)
- [**iconifydl**](https://github.com/ksckaan1/iconifydl) — Go (3 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
