# umwl-tui — specification

Full-screen terminal application (Go, Bubble Tea) that loads proposed unmanaged workloads and IP lists into an
Illumio PCE through `workloader`, reconciling every row by IP against the live inventory and never writing before an
explicit confirmation. This document is the contract for anyone (human or Claude Code) extending it.

## 1. Goals and non-goals

Goals

- One binary, no runtime dependencies besides the `workloader` binary in the working folder or PATH.
- Every PCE interaction goes through `workloader` subcommands (never the REST API directly) so the audit trail is
  workloader's own logs plus this tool's `runs/<timestamp>/` folder.
- A user who has never seen the CSV can still run the load safely: the interface shows what will happen, asks per row
  where the PCE already has an object, dry-runs, and only then writes in batches with retry/skip.
- Everything decided or observed is persisted: CSVs per bucket, chunk CSVs, workloader logs, `report.md`, `report.json`.

Non-goals

- Editing policy (rules, rulesets, services). Only workloads (unmanaged), their labels and IP lists.
- Managing `pce.yaml` beyond `pce-add --api-key`. Secrets are never stored by this tool.
- Replacing the flow analysis: the CSV is produced upstream (see the README prompt).

## 2. Architecture

```
cmd/umwl-tui/main.go        flags → tui.Run(Config)
internal/engine/            pure logic, no terminal I/O
  csv.go                    CSV contract (LoadCSV, WriteCSV, ParseIPs, priority filter, duplicate handling)
  workloader.go             Workloader wrapper (Run streams stdout/stderr lines; PCEList; PCEAdd; ConnTest; DownloadRelease)
  pce.go                    Inventory (wkld-export/label-export/label-dimension-export), PlanLabels, Classify, Apply, Split, Report
  exec.go                   DryRun, MakeChunks, RunChunk, Verify, IPLDry/IPLImport
internal/tui/               Bubble Tea model
  model.go                  Model, Init/Update/View, layout, status bar, sidebar, key bar, help
  steps.go                  one section per step: view, keys, key handler, async commands, result handling (onWork)
  modal.go                  choice / info / form modals (capture all keys while open)
  picker.go                 Midnight-Commander-style file chooser (path field + listing)
  util.go                   table renderer, cell-width-aware truncate/pad/wrap/clip
  styles.go                 palette (Illumio orange #FF5500, ink #313638, paper #F7F4EE) and glyphs
testdata/                   mock workloader (python) + example CSVs used by the tests
```

Rules that keep this maintainable

- `engine` must stay free of Bubble Tea imports and of `fmt.Print*`; it returns values, the TUI renders them.
- Long-running work runs inside `tea.Cmd` functions; results come back as typed messages handled in `onWork`.
  The model is never mutated from a goroutine.
- workloader output reaches the log pane through `Workloader.Out` → buffered channel → `logMsg`. Never block on `Out`.
- Geometry is integer cells; every panel is `clip`ped to its box so the frame never grows past the terminal.

## 3. State machine

`step` 0..10, `status[step]` ∈ pending | active | running | done | failed | skipped.

| # | Step | Enters when | Async work | Leaves with |
|---|---|---|---|---|
| 0 | Preflight | start | FindBinary, `version`, `pce-list`, `label-dimension-export` (conn test) | enter (all 3 checks ok) |
| 1 | Load CSV | enter from 0 | LoadCSV (validation, priority filter) | enter |
| 2 | PCE inventory | enter from 1 | wkld-export, label-export, label-dimension-export | automatic → 3 |
| 3 | Labels | inventory done | — | enter (no unknown label columns) |
| 4 | Reconcile by IP | enter from 3 | — | enter (all non-NEW rows decided, or skip-all modal) |
| 5 | Review new | decisions made | — | enter |
| 6 | Dry run | enter from 5 | wkld-import without --update-pce (create + update CSVs) | enter+y → 7 · x → 10 (aborted) |
| 7 | Execute | confirmed | one wkld-import --update-pce --no-prompt per chunk | automatic → 8; failed chunk → modal r/s/q |
| 8 | Verify | chunks done | wkld-export again; every created IP must be present | enter |
| 9 | IP lists | --ipl given | ipl-import dry run; enter+y → ipl-import --update-pce | enter / n |
| 10 | Report | always last | report.md + report.json | enter/q quits |

Row states (`Row.Review`): `NEW`, `EXISTS-UNMANAGED`, `CONFLICT-MANAGED` (IP belongs to a VEN workload),
`CONFLICT-MULTIPLE` (several objects share the IP) → after a decision: `UPDATE-<kind>`, `SKIPPED-<kind>`, `NEW-DUP`,
`SKIPPED-USER` (review step).

Update semantics: the row is rewritten with `href` of the PCE object, `hostname` from the PCE, `interfaces` blank
(workloader leaves interfaces untouched when blank), `name` from the PCE unless renamed. This is what makes a plain
`wkld-import` (no `--umwl`) update labels and description only.

## 4. UI contract

Layout (fixed chrome, integer cells): status bar (1 line) · sidebar 28 cols (steps + events) · main panel · log pane
(height/4, 6..14 lines) · key bar (1 line). Focus toggles with `tab` between the main panel and the log.

Global keys: `tab` focus log · `?` help · `q` quit (confirmation once work started) · `ctrl+c` quit.
Modals capture every key; `esc` cancels a form or picks the cancel option of a choice. A handler may open another dialog from inside a dialog (chained pickers/forms): the caller only clears the dialog it dispatched to.

File chooser (`picker.go`): path field on top (`tab` or `/` to edit; enter on a directory jumps there, on a file selects it; `~` expands), listing below with `..`, directories first, preferred extensions (`.csv`) next and highlighted, other files dimmed; `←`/backspace = parent, enter/`→` = open or select, `esc` = cancel (or "none" for optional pickers). Used when no CSV is given on the command line and after a failed CSV load (which rewinds to step 0 and reopens the chooser).

Per-step keys are listed in the bottom bar and in `stepKeys()`. Tables: `↑↓ j k pgup pgdown g G`.

Colors: status glyphs ✔ ● ▶ ○ ✖ –; row badges NEW (green), EXISTS/UPDATE (blue), CONFLICT (red), SKIPPED (grey);
destructive confirmations use the red title (`Modal.Danger`).

Minimum terminal: 80×24. Everything must render at that size without wrapping the frame (see `TestViewFitsTerminal`).

## 5. Engine contract with workloader (verified against v12.1.9)

- `wkld-import` matches on ONE column, chosen by `--match` (href | hostname | name | external_data) or, when the flag is
  absent, by priority over the columns present in the CSV: href, then hostname, then name. Rows whose match column is
  blank are skipped (`the match column cannot be blank`) — there is no fallback from hostname to name. Never IP, hence
  the inventory index by IP in this tool. Consequences: the create pass passes `--match name` whenever a row has a
  blank hostname (`engine.CreateArgs`), the update pass always passes `--match href` (`engine.UpdateArgs`), and a dry
  run containing `cannot be blank` or `nothing to be done` is reported as not ok (`hasProblem`).
- `--umwl` creates unmanaged workloads for rows not found; `--update=false` prevents accidental updates on the create pass.
- `--update-pce` writes; `--no-prompt` skips workloader's own confirmation (this tool already asked).
- `pce-add --api-key` reads `--api-user/--api-secret/--org`; without `--api-key` workloader asks email/password (never used here).
- Config lookup: `--config-file` → `$ILLUMIO_CONFIG` → `./pce.yaml`. The tool passes `--pce <name>` only when given.
- Log format: `YYYY-MM-DD HH:MM:SS [LEVEL] - message`; the tool greps `to be created|to be changed|\[WARN|\[ERROR|created new .* label|bulk (create|update) workload successful`.

## 6. Testing

- `go test ./...` runs headless: `tui_test.go` drives the model with synthetic key messages and executes commands
  synchronously (`drain`), against `testdata/mock-workloader.py`. Add a scenario for every new decision path.
- `TestViewFitsTerminal` guards geometry at 80×24 up to 160×40.
- `UMWL_DUMP=<dir> go test ./internal/tui -run TestDumpScreens` writes ANSI screenshots of each step for review.
- Manual: `make build && ./umwl-tui --setup-only` in a folder with the mock as `./workloader`.

## 7. Security requirements and tests

Threat model: the tool runs on an engineer's laptop with a write-capable PCE API key in `./pce.yaml` and customer
data in CSVs. Risks: leaking the API secret, writing to the wrong PCE, command injection through CSV content, tampered
downloads.

Requirements (each one has, or must get, a test or a review item):

1. Secrets: the API secret is passed to `pce-add` as an argv element, never through a shell, never written to the log
   pane, `runs/`, `report.*` or the events list (`PCEAdd` masks it). Test: grep the run folder and the log buffer for
   the secret after a mocked `pce-add`.
2. No shell: every external call is `exec.Command(bin, args...)`; CSV cells are never interpolated into a command line.
   Test: a CSV with `$(id)`, backticks and `;` in name/description must round-trip unchanged into the chunk CSVs.
3. Wrong-PCE guard: preflight shows `pce-list` and the working folder; the report records the PCE name and cwd.
   One working folder per Illumio account + PCE is documented, and `pce.yaml` is gitignored.
4. Least writes: nothing calls `--update-pce` before step 7, and step 7 only after the explicit modal (`Danger`).
   Test: `TestQuitBeforeExecuteWritesReport` asserts no created rows when stopping at the dry run.
5. Downloads: `DownloadRelease` uses `curl -fsSL` over HTTPS to github.com only, then strips quarantine. workloader
   releases publish no checksums; document that the user may verify the binary out of band, and never auto-download
   without the confirmation modal.
6. File permissions: run folders 0755, CSVs 0644 — they contain customer IPs but no secrets. `pce.yaml` is created by
   workloader itself (its permissions are workloader's responsibility; document `chmod 600 pce.yaml`).
7. Dependencies: pinned in `go.mod`; `make security` runs gosec and govulncheck. CI should fail on HIGH findings.
8. Input hardening: CSV parser is `encoding/csv` with `LazyQuotes`; IPs validated with `net.ParseIP`; oversized cells
   are truncated only for display, never for the import file.

## 8. Backlog (ordered)

1. Column filter/search in the reconcile and review tables (`/` to filter by IP, name or state).
2. Persist decisions to `runs/<ts>/decisions.json` and offer to replay them on a re-run of the same CSV.
3. Mouse support for row selection and modal buttons (Bubble Tea `tea.WithMouseCellMotion`).
4. `--yes` non-interactive mode for CI (accept defaults: update EXISTS, skip CONFLICT, execute) with the same report.
5. Windows terminal validation (the code paths exist; untested).
6. Optional: read-only "diff" screen after Verify showing labels before/after per updated href.
