# CLAUDE.md — illumio-workloader-import-kit

Working notes for Claude Code (and humans) developing this repository.

## What this is

Tools to load the unmanaged workloads and IP lists proposed by an Illumio flow analysis into a PCE, on top of
`brian1917/workloader`:

- `umwl-tui` (Go, Bubble Tea) — **the product**: the full-screen application. Source in `cmd/umwl-tui` and `internal/`.
  Its preflight installs workloader by **building it from source** (`b`: clone into `workloader-src/`, native
  `go build`), with the Intel-only release download as fallback (`d`) and a saved path to a binary elsewhere
  (`w`, `umwl-tui.json` / `~/.config/umwl-tui/config.json`).
- `anonymize_export.py` — pseudonymize exports before the AI analysis / restore names in the proposals (stdlib).
- `legacy/umwl_loader.py`, `legacy/reconcile_umwl.py` — the earlier plain-terminal implementation (Python stdlib),
  kept as fallback and readable reference; new features go to `umwl-tui` first and are back-ported only if cheap.
- `examples/` — CSVs from real POCs (customer IPs; templates only). `docs/` — the merged guide (ES/EN, branded PDF +
  HTML), `SPEC-umwl-tui.md`, `prompts/` (Claude Project instructions + per-conversation prompt), `img/` schematics.

Read `docs/SPEC-umwl-tui.md` before changing `umwl-tui`: it is the contract (state machine, UI, engine, security).

## Build, test, ship

```bash
make build      # go mod tidy + build ./umwl-tui (Go 1.24+; first build downloads modules)
make test       # go test ./... — headless TUI tests drive the model with synthetic keys against testdata/mock-workloader.py
make vet        # gofmt -l + go vet (must print nothing)
make security   # gosec + govulncheck
make dist       # cross-compile darwin/arm64, darwin/amd64, linux/amd64, linux/arm64 into dist/ + SHA256SUMS
UMWL_DUMP=/tmp/shots go test ./internal/tui -run TestDumpScreens   # ANSI screenshots of every step
```

Release: bump the tag, `make dist`, `gh release create vX.Y.Z dist/* --title vX.Y.Z --notes "..."`. Binaries are
never committed (`dist/` is gitignored). macOS users must `xattr -d com.apple.quarantine umwl-tui` after download.

Python side: `python3 -m py_compile legacy/umwl_loader.py legacy/reconcile_umwl.py anonymize_export.py`; run the loader against the mock: copy
`testdata/mock-workloader.py` to `./workloader` in a temp folder with a CSV and an empty-ish `pce.yaml`.

## Conventions

- `internal/engine` never imports Bubble Tea and never prints. All PCE access goes through `workloader` subcommands
  via `exec.Command(bin, args...)` — no shells, no string-built command lines, no direct REST calls.
- Async work = `tea.Cmd` returning a typed message handled in `onWork`. Never mutate the model from a goroutine.
- Every new decision path gets a scenario in `internal/tui/tui_test.go`; every engine function gets a unit test.
- Geometry: integer cells, panels clipped (`clip`), 80×24 must render (guarded by `TestViewFitsTerminal`).
- Language: code, comments, UI strings and README.md in English; README.es.md and the Spanish guide mirror them.
  Customer-facing deliverables produced *with* the kit stay in Spanish (LATAM usage).
- Label nomenclature used in the examples: `R_`/`A_`/`E_`/`L_` prefixes; workload name = "Role IP"; hostname blank;
  interfaces `eth0:<ip>`; description `[<grupo> P<prio> conf:<Alta|Media|Baja>] evidence`.
- One working folder per Illumio account + PCE (workloader reads `./pce.yaml`). Never commit `pce.yaml`, `workloader`
  binaries, `runs/`, `dist/`, logs (see `.gitignore`).
- Verify workloader flags against the source (github.com/brian1917/workloader, verified: v12.1.9) before documenting them.
  The build ldflags mirror its `.github/workflows` (`-X github.com/brian1917/workloader/utils.Version=$(cat version)`);
  re-check them when bumping the verified version.
- Never commit `workloader-src/` or `umwl-tui.json` (gitignored). Settings files hold only the workloader path.

## Security testing checklist (run before every release)

1. `make security` clean of HIGH findings (gosec, govulncheck).
2. `go test ./...` includes: secret masking in `PCEAdd` echo and `Calls` (`TestPCEAddMasksSecret`); hostile CSV cells
   round-trip as data (`TestCSVRoundTripHostileCells`); no writes before Execute (`TestQuitBeforeExecuteWritesReport`).
3. Manual: run `umwl-tui --setup-only` against a mock, then `grep -r <secret> runs/` must find nothing.
4. Manual: confirm the "Write to the PCE?" modal is the only path to `--update-pce` (`grep -n "update-pce" internal/`).
5. Review any new `exec.Command` call: binary from `FindBinary`/curl/unzip/xattr/git/go/brew only; arguments never from
   CSV cells or free text except as file paths this tool wrote itself (the build tag comes from `LatestTag`).
   `TestWorkloaderPathSetting` covers the settings precedence and the execute-bit check; `TestBuildOfferWithoutGo`
   covers the no-toolchain path.
6. Dependencies: `go.mod` pinned; review `go.sum` diffs in PRs; no new module without a reason in the PR description.

## Prompts that work well here

- "Implement backlog item N from docs/SPEC-umwl-tui.md; add the test scenario first, keep engine UI-free, run make vet test."
- "Reproduce this screenshot/bug: <paste>. Add a failing test in tui_test.go that captures it, then fix."
- "Audit internal/ against section 7 of the spec and report gaps as a table before changing code."
