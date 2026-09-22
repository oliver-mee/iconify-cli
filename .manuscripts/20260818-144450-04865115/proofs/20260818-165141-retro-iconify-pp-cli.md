# Printing Press Retro: Iconify

## Session Stats
- API: iconify (api.iconify.design, no auth)
- Spec source: hand-authored internal YAML from official docs, all 9 endpoints probed live
- Scorecard: 92/100 (Grade A)
- Verify pass rate: 100%
- Live dogfood: 106/106 at full level
- Fix loops: 2 shipcheck rounds plus a polish pass
- Manual code edits: 7 hand-authored files (corpus schema, index package, 7 novel commands)
- Features built from scratch: 7 (index, set-pick, shadcn, kit, audit, diff, swap)

Ran in the same conversation as the generation, so session evidence is available.

## Findings

### 1. `response_format` is ignored; every non-JSON endpoint command is broken (Bug)

- **What happened:** The spec declared `response_format: binary` on the SVG and CSS
  endpoints. The generated commands still ran the response through a JSON assertion, so
  the SVG's leading `<` matched the HTML branch and the command failed with
  `not authenticated or session expired; API returned HTML instead of JSON`. Both the
  flagship icon fetch and the CSS generator were unusable as generated.
- **Scorer correct?** N/A, not a score penalty. Verify and dogfood both passed the
  commands because they only exercised `--help` and dry-run paths.
- **Root cause:** Generator. `internal/cli/data_source.go` emits
  `resolveReadWithStrategyResponsePathAndJSONGuard(..., guardLiveJSON bool, ...)`, so the
  plumbing to skip the assertion already exists. But the endpoint template calls the
  wrapper `resolveReadWithStrategyAndResponsePath(...)`, which hardcodes `true`:

  ```go
  func resolveReadWithStrategyAndResponsePath(...) (...) {
      return resolveReadWithStrategyResponsePathAndJSONGuard(..., true, hintWriter)
  }
  ```

  So `response_format` never reaches the guard it was presumably added for.
- **Cross-API check:** Any endpoint declaring a non-JSON `response_format`. The internal
  spec format documents four values (`json | csv | html | binary`); three are non-JSON.
- **Frequency:** subclass:non-json-response-format
- **Fallback if the Printing Press doesn't fix it:** Poor. The failure text says
  "not authenticated or session expired", which points an agent at auth rather than at
  response handling. I lost time chasing auth before reading the assertion source.
- **Worth a Printing Press fix?** Yes. The guard parameter already exists, so the fix is
  wiring, not new machinery. A JSON-declaring endpoint is unaffected.
- **Inherent or fixable:** Fixable.
- **Durable fix:** In the endpoint command template, call the `...AndJSONGuard` variant
  and pass `guardLiveJSON=false` when the endpoint's `response_format` is anything other
  than `json`. Do not change the default for JSON endpoints.
- **Test:** Positive — an endpoint with `response_format: binary` returns its raw body and
  exits 0. Negative — an endpoint with `response_format: json` (or unset) still fails on a
  non-JSON body, preserving the existing auth-detection behaviour.
- **Evidence:** `iconify` endpoints `icons get` (`/{prefix}/{name}.svg`) and `icons css`
  (`/{prefix}.css`), both declared `response_format: binary`, both failed on every live
  invocation. Worked around with hand-authored raw-body handlers.
- **Step B (three named APIs):** Honestly, only two.
  1. `iconify` — verified broken, this run.
  2. `library/developer-tools/claude-agent-sdk-python-docs` — declares
     `response_format: binary` on 9 endpoints serving `llms.txt` and `.md` docs. Its
     generated tree contains **no `assertLiveJSONBody` at all**, so it predates the
     assertion and would break the same way on reprint under 4.30.3.
  3. No third published CLI uses a non-JSON `response_format`; the whole library has 9
     occurrences and they all belong to that one CLI.

  Recording this shortfall rather than padding it. The case for filing anyway: this is a
  documented spec field that is silently ignored, the only published user of it predates
  the regression, and the counter-check below is clean.
- **Step C (counter-check):** Would the fix hurt an API without this pattern? No. It only
  relaxes an assertion for endpoints that explicitly declare a non-JSON response. JSON
  endpoints keep the current behaviour, including the auth-detection heuristic.
- **Step D (recurrence):** No prior retro matches; this is the first manuscripts dir on
  this machine.
- **Step G (case against):** A maintainer could say `response_format` is only honoured on
  the client side, the field is barely used across the library, and a CLI serving binary
  bodies should hand-write its handlers anyway. That is the strongest counter, and it
  fails because the generator already carries the `guardLiveJSON` parameter: the intent to
  support this is in the code, the wiring is just absent, and the failure mode
  misdiagnoses itself as an auth problem.
- **Related prior retros:** None.

### 2. `crowd-sniff` fails for every API (Bug)

- **What happened:** `crowd-sniff` could not discover endpoints for any API tried. The user
  explicitly asked for it as an enrichment pass and it could not be delivered.
- **Scorer correct?** N/A.
- **Root cause:** Binary, `crowd-sniff` command. Two distinct symptoms:
  - `--api api.iconify.design`, `--api iconify`, `--api stripe`, `--api github` all end in
    `crowd-sniff: downloads API returned status 400` followed by
    `no endpoints discovered`.
  - `--api notion` fails differently: `base URL must use HTTPS: http://127.0.0.1:${port}`,
    which looks like a localhost URL scraped out of package source being accepted as a
    base URL candidate.
- **Cross-API check:** Every API attempted, across two very different failure modes.
- **Frequency:** every API
- **Fallback if the Printing Press doesn't fix it:** None. The subcommand is the fallback.
  When it fails there is nothing to fall back to except docs-only discovery.
- **Worth a Printing Press fix?** Yes. A whole discovery subcommand is non-functional.
- **Inherent or fixable:** Fixable. The 400 suggests the npm downloads endpoint contract
  changed; the notion case is a separate input-validation gap.
- **Durable fix:** Two parts, and they are probably independent.
  1. Fix or replace the npm downloads call that returns 400. If npm's API changed shape,
     the request needs updating; if the endpoint is being called with an unexpected
     package name, the name derivation needs a guard.
  2. Reject non-HTTPS and loopback candidates during base-URL derivation rather than
     surfacing them as a fatal error to the user.
  I could not isolate which layer produces the 400 without the press source to hand, so
  an implementer should confirm against `crowd-sniff`'s npm client before committing to
  a fix.
- **Test:** Positive — `crowd-sniff --api stripe` discovers at least one endpoint, or
  fails with a message naming what it could not reach. Negative — a genuinely unknown API
  still reports "no endpoints discovered" rather than a transport error.
- **Evidence:** Four invocations this session, all failing:
  `--api api.iconify.design`, `--api iconify`, `--api stripe`, `--api github` (400);
  `--api notion` (HTTPS validation).
- **Step B (three named APIs):** stripe, github, iconify, all reproducible with the
  command above. notion as a fourth with a different symptom.
- **Step C (counter-check):** No API is helped by the current behaviour.
- **Step D (recurrence):** No prior retro matches.
- **Step G (case against):** A maintainer could say npm's API is upstream and outside the
  press's control. That fails because the failure is total and silent-ish: the command
  exits 0 while reporting no endpoints, so an agent can read it as "this API has no
  community SDKs" rather than "discovery is broken".
- **Related prior retros:** None.

## Prioritized Improvements

### P2 — Medium priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|---|---|---|---|---|---|
| F1 | `response_format` ignored for non-JSON endpoints | generator | subclass:non-json-response-format | Poor, error misdiagnoses as auth | small | Only relax the guard when `response_format != json` |
| F2 | `crowd-sniff` discovers nothing for any API | scorer/binary | every API | None, it is itself the fallback | medium | Keep "no endpoints" distinct from "discovery failed" |

### Skip
| Finding | Title | Why it didn't make it |
|---|---|---|
| C3 | Live-check probes race each other through a shared HOME, so a reader probe runs while the writer probe is still populating the store | Step B: only one API named. Real, and polish reproduced it, but I cannot point to a second CLI with a writer-plus-local-reader probe shape. |
| C4 | Generated `feedback` parent shipped without an Examples section, failing its own dogfood help probe | Step B: 24 of 25 sampled published CLIs *do* have it, so this looks version-specific rather than systemic. Not enough evidence to claim a live template gap. |

### Dropped at triage
| Candidate | One-liner | Drop reason |
|---|---|---|
| Hollow coverage counts dry-run-only passes against mutating commands | Blocks publishing any CLI whose command writes files | raised-elsewhere: already open as #4046; commented there with the mutating-command data point rather than filing a duplicate |
| `sync` warns on a resource that requires a parameter and is not listable | The `keywords` endpoint needs `prefix` or `keyword` | printed-CLI: spec shape, fixed by not declaring it syncable |
| Root `--timeout` default aborted a long fan-out command | 1 minute killed a 236-set index | printed-CLI: the novel command owns its own deadline |
| Novel commands errored on an unbuilt local store | Should return an empty result plus a hint | printed-CLI: the SKILL already documents the missing-mirror pattern; I had not applied it |

## Work Units

### WU-1: Honour `response_format` when emitting endpoint read calls
- **Stable ID:** WU-1
- **Priority:** P2
- **Type:** bug
- **Component:** generator
- **Goal:** An endpoint declaring a non-JSON `response_format` returns its raw body instead of failing a JSON assertion.
- **Target:** The endpoint command template that emits `resolveReadWithStrategyAndResponsePath(...)`, plus the `data_source.go` wrapper in `internal/generator/`.
- **Acceptance criteria:**
  - positive test: a generated command for an endpoint with `response_format: binary` returns the raw body and exits 0 against a server returning `image/svg+xml`.
  - negative test: a generated command for a `json` endpoint still returns the existing error when the body is HTML, preserving auth detection.
- **Scope boundary:** Does not change how the body is rendered or written to disk, and does not add a `--output` flag. Only whether the JSON assertion runs.
- **Dependencies:** None
- **Complexity:** small

### WU-2: Repair `crowd-sniff` discovery
- **Stable ID:** WU-2
- **Priority:** P2
- **Type:** bug
- **Component:** scorer
- **Goal:** `crowd-sniff` either discovers endpoints or fails with a message that distinguishes "no community SDKs" from "discovery is broken".
- **Target:** The `crowd-sniff` command's npm client and base-URL derivation.
- **Acceptance criteria:**
  - positive test: `crowd-sniff --api stripe` returns at least one endpoint, or an error naming the unreachable dependency.
  - negative test: an API with genuinely no npm presence still reports "no endpoints discovered" and exits non-zero, so a caller cannot mistake breakage for absence.
- **Scope boundary:** Does not extend crowd-sniff to new sources; only restores the npm and GitHub paths it already claims.
- **Dependencies:** None
- **Complexity:** medium

## Anti-patterns
- The JSON assertion's message names auth as the cause of a response-shape problem. An
  error that misdiagnoses itself costs more than one that simply says less.
- `crowd-sniff` exits 0 when it discovers nothing, so a total failure reads as a negative
  result. Discovery tools should distinguish "looked and found nothing" from "could not look".

## What the Printing Press Got Right
- The live dogfood matrix caught four real defects that inspection had missed, including a
  22% silent data loss in a hand-written sync path that looked healthy from the outside.
- `pp:happy-args` worked for every skipped positional command I applied it to, including
  the "fewer segments than placeholders" case, which the currently open #4046 reports as
  unsupported. Worth confirming, because it may narrow that issue considerably.
- The polish pass's output review found two genuine wrong-answer bugs, not style nits: a
  cross-set migration reporting false coverage, and a lookup reporting real icons as
  missing. Both were logic errors in hand-written code that all structural gates passed.
- `regen-merge` preserved all ten hand-authored files across a `generate --force`, exactly
  as documented.
