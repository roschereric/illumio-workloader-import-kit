Español: [README.es.md](README.es.md)

# illumio-workloader-import-kit

`umwl-tui` is a full-screen terminal application (Go, single static binary) that loads into an Illumio PCE the **unmanaged workloads (UMWL)** and **IP lists** proposed by a flow analysis, using [workloader](https://github.com/brian1917/workloader) as the import engine: it reconciles every row by IP against the live inventory, dry-runs, and writes in batches only after an explicit confirmation. Around it, the repository documents the whole method to go from an Explorer/Traffic export to loaded objects: what to export from the PCE, how to pseudonymize it, the Claude Project instructions that produce the report and the CSVs, the naming convention, and the workloader CSV contract. The earlier plain-terminal Python loaders live in `legacy/` as a fallback.

> Verified against **workloader v12.1.9** (release of June 12, 2025; code on `master` consulted on September 3, 2026) and **Illumio Core 24.x/25.x** (documentation at `product-docs-repo.illumio.com`). The command names, flags and CSV headers in this README were taken from the workloader source code (`cmd/root.go`, `cmd/wkldimport`, `cmd/iplimport`, `cmd/wkldexport`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension`, `cmd/traffic`). If you use another version, confirm with `./workloader <command> --help`.

## Quick start

1. Working folder (one per Illumio account + PCE, next section; every command below runs there): `mkdir -p ~/illumio/customer-a/scp57-org12 && cd ~/illumio/customer-a/scp57-org12`
2. The kit (private repository: `gh auth login` or an SSH key loaded into GitHub first): `git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit`
3. The binary: `gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS && shasum -a 256 -c SHA256SUMS --ignore-missing && mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui` (or build it: `(cd kit && make build) && cp kit/umwl-tui .`).
4. workloader + `pce.yaml` + connection test: `./umwl-tui --setup-only` (downloads workloader, runs `pce-add --api-key`, tests with `label-dimension-export`).
5. Export from the PCE (console: guide §4 "Exporting from the PCE"; or workloader): `./workloader traffic --start 2026-08-17 --end 2026-09-01 --max-results 200000 --output-file TrafficData.csv`, then `./workloader wkld-export --output-file pce-workloads.csv`, `./workloader label-export --output-file pce-labels.csv`, `./workloader label-dimension-export --output-file pce-label-types.csv`.
6. Pseudonymize before sharing: `python3 kit/anonymize_export.py anon TrafficData.csv -o TrafficData.anon.csv --map anon-map.json --customer "Customer A" --domain customer-a.com`
7. Analysis in a Claude Project: instructions = `kit/docs/prompts/context.md`, message = `kit/docs/prompts/prompt-short.md`, attachments = `TrafficData.anon.csv` + the `pce-*.csv` exports. It returns the report, `<group>-umwl-import.csv` and `<group>-ipl-import.csv`.
8. Real names back into the proposal: `python3 kit/anonymize_export.py deanon C4-umwl-import.csv -o customer-a-umwl-import.csv --map anon-map.json` (same for the IP lists CSV if it carries FQDNs).
9. Load: `./umwl-tui customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1` — ten steps, dry run, "Write to the PCE?" confirmation, batches, verification.
10. The run report: `runs/<timestamp>/report.md` (and `report.json`, the CSVs per bucket and every workloader log).

## Important: one working folder per Illumio account (organization) + PCE

workloader resolves the PCE connection by reading, in this order, `--config-file`, the `ILLUMIO_CONFIG` variable and, if neither is set, **`./pce.yaml` in the working folder**. `umwl-tui` also writes `runs/<timestamp>/` in the working folder, with the inventory exported from the PCE, the final CSVs and the logs of every batch, and `anonymize_export.py` keeps `anon-map.json` there.

The isolation unit is neither "the customer" nor "the PCE" but each **Illumio account or organization + PCE** combination, that is, each distinct `pce.yaml` profile (FQDN, port, org id and API key):

- An account can have several PCEs (SaaS and on-prem, production and DR, regions): each one is a folder.
- A SaaS PCE (one FQDN) hosts several organizations; each `org id` is a separate tenant and gets its own folder, even when the FQDN is the same. The `org id` is the number that appears in the console URL (`…/orgs/<id>/…`).
- Another account: same layout, never the same `pce.yaml`.

If the `pce.yaml` files of two tenants share a folder (or a `pce.yaml` holds several PCEs and you forget `--pce`), the risk is concrete: **importing one customer's objects into another customer's PCE or organization**. The rule is simple: **one working folder per account + PCE**, with its own `pce.yaml`, its CSVs and its `runs/`. Suggested naming: `~/illumio/<account>/<pce-or-org>/`.

```
~/illumio/
├── customer-a/
│   ├── scp57-org12/                  # account customer-a, SaaS PCE scp57, org 12
│   │   ├── umwl-tui                  # the application (release binary or make build)
│   │   ├── workloader                # binary; umwl-tui downloads it (or a symlink to a shared one)
│   │   ├── pce.yaml                  # ONLY this account + PCE; created by pce-add; never copy it
│   │   ├── anon-map.json             # pseudonym map; never leaves this folder
│   │   ├── customer-a-umwl-import.csv
│   │   ├── customer-a-ipl-import.csv
│   │   ├── runs/20260903-101500/     # one subfolder per run
│   │   └── kit/                      # clone of this repository (docs, prompts, anonymizer, examples)
│   └── onprem-prod/                  # same account, another PCE: another folder, another pce.yaml
└── customer-b/<pce-or-org>/          # another account, same layout
```

Before every run, `umwl-tui` executes `workloader pce-list` and shows you the PCE it is going to use; read that name and the FQDN before accepting. If the `pce.yaml` has more than one PCE, always pass `--pce <name>`. workloader supports several profiles in a single `pce.yaml` (with `default_pce_name` and the `--pce` flag), but the kit deliberately does not rely on that: a forgotten `--pce` or a wrong default would load the objects into the wrong tenant.

## Repository contents

| Path | What it is |
|---|---|
| `cmd/umwl-tui/`, `internal/`, `go.mod`, `Makefile`, `testdata/` | **`umwl-tui`** (Go, Bubble Tea): the full-screen application. `internal/engine` is the workloader/CSV logic with no terminal I/O; `internal/tui` the model, steps, modals and file chooser; `testdata/` the mock workloader and CSVs the headless tests use. `make build`, `make test`, `make dist`. |
| `anonymize_export.py` | Consistent, reversible pseudonymization of exports (`anon`) and restoration of the real names in the proposed CSVs (`deanon`). Python 3, standard library. |
| `docs/Guide-umwl-tui-EN.{pdf,html}`, `docs/Guia-umwl-tui-ES.{pdf,html}` | The merged guide (30 pages, English and Spanish): workflow, requirements, initialization, exporting from the PCE with schematic screens, anonymization, the analysis with Claude, umwl-tui step by step, v2 cycle, troubleshooting. |
| `docs/prompts/context.md`, `docs/prompts/prompt-short.md` | Claude Project instructions (the full method) and the per-conversation message (v1 and v2). |
| `docs/export-columns.txt`, `docs/img/*.svg` | Header of a 25.x Traffic export; the schematics used by the guide. |
| `docs/SPEC-umwl-tui.md`, `CLAUDE.md` | The contract of `umwl-tui` (state machine, UI, engine/workloader contract, security requirements, backlog) and the build/test/release commands, conventions and security checklist for Claude Code sessions. |
| `examples/*-umwl-import.csv`, `examples/*-ipl-import.csv` | Proposed CSVs from a proof of concept (customer lab IPs: format templates, not for loading as-is). `cliente3-*-v2.csv` are the **recommended format**: `R_/A_/E_/L_` labels, `name` = role + IP, empty `hostname`, `interfaces` = `eth0:<ip>`, `IPL_<Site>_<Use>` lists. |
| `legacy/` | `umwl_loader.py` and `reconcile_umwl.py`, the plain-terminal predecessors (see "Legacy" below and `legacy/README.md`). |
| `.gitignore` | Excludes `workloader`, `umwl-tui`, `pce.yaml`, `runs/`, `*.log`, `anon-map.json`, `*.anon.csv`, `*.real.csv`, `dist/`. **Never push `pce.yaml`: it contains the API key.** |

## Requirements

- **macOS** (tested on Apple Silicon; the flow is the same on Intel and on Linux). `umwl-tui` ships as a static binary for darwin/arm64, darwin/amd64, linux/amd64 and linux/arm64; building it needs **Go 1.24+**. **Python 3.9+** (standard library only) for `anonymize_export.py` and the legacy loaders.
- **workloader v12.x**. No installation required: it is a binary. The macOS release is published as `mac-<version>.zip`; on Apple Silicon it runs through Rosetta 2. `umwl-tui --setup-only` downloads it, or builds it from source if Go is installed.
- A **PCE API key with write permissions**. The key inherits the role of the user who creates it: if the user is read-only, `wkld-import --update-pce` fails. To create it: user menu (top right) → **My API Keys** → **Add**; save the **Authentication Username** (`api_…`) and the **Secret**, which are shown only once. A service account with equivalent permissions also works. **Network access to the PCE over HTTPS** (port 443 on SaaS, typically 8443 on-prem). On SaaS the `org id` is the number that appears in the console URL (`…/orgs/<id>/…`).

## Initializing the working folder

Done once per account + PCE combination; afterwards every load is a single command. Steps 1–4 of the quick start are the short form.

1. **Folder**: `mkdir -p ~/illumio/customer-a/scp57-org12 && cd ~/illumio/customer-a/scp57-org12`. Everything that follows runs with that folder as the current directory: `./umwl-tui`, `./workloader`, `./pce.yaml` and `./runs` sit next to the CSVs, not inside the clone.
2. **Kit**: `git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit` (equivalent: `gh repo clone …`; without git: Code → Download ZIP, unpacked as `kit/`). The CSVs live in the working folder, not in `kit/`; `kit/examples/` contains templates only. Update with `git -C kit pull`. To share a single clone between folders: `ln -s ~/illumio/kit-shared kit`. Then `umwl-tui` itself: release binary or `make build` (see "Install" below).
3. **workloader and pce.yaml**: `./umwl-tui --setup-only`. It looks for `./workloader` (then the PATH, or the `--workloader` path). If it does not find it, it offers to download the release (`mac-<version>.zip`, with `curl` + `unzip` and `xattr -d com.apple.quarantine`) or to clone the workloader repository into `workloader-src/` and build it with `go build`. Then it runs `pce-list`; if there is no `pce.yaml`, or workloader replies "no pce configured", it goes straight to `pce-add`: it asks for a short name, FQDN, port, API user, API secret and org id, runs `pce-add --api-key …` with those values (so workloader never asks for email and password), shows `pce-list` again and tests the connection with a `label-dimension-export`. (Alternative without the binary: `python3 kit/legacy/umwl_loader.py --setup-only` does the same step.)
4. **Sanity check (read-only)**: `./workloader pce-list && ./workloader wkld-export --output-file sanity.csv` and look at the hostnames: is this the inventory of the expected tenant? A read-only export proves that the API key works before anything is written.
5. **Another account or another PCE**: repeat in a new folder. Never copy `pce.yaml`. Several PCEs in one `pce.yaml` is not the kit's scheme; if it exists anyway, always pass `--pce <name>`.

## umwl-tui

The same ten-step flow the kit always had, as a real terminal application (htop / Midnight Commander style): a status bar with the PCE, the run folder and the step counter; the list of steps with their state on the left; the active step's panel on the right (tables you browse with the arrow keys, side-by-side "proposed vs. in the PCE" detail, progress bars); the live workloader output at the bottom; and a key bar that changes per step. Every decision that matters (conflicts, writes to the PCE, failed batches) is a modal; nothing is written before the "Write to the PCE?" confirmation after the dry run.

```
  umwl-tui  ● Ready                          PCE default (pce.yaml)  │  run 20260903-042451  │  step 4/10
╭──────────────────────────╮╭────────────────────────────────────────────────────────────────────────────╮
│ STEPS                    ││ ■ Reconcile by IP — 42 rows · 3 awaiting a decision                        │
│ ✔  0 Preflight           ││   IP                PROPOSED NAME            STATE               IN THE PCE│
│ ✔  1 Load CSV            ││ ▶ 10.43.43.21       Zabbix Server 10.43.43…  EXISTS-UNMANAGED    zbx-old   │
│ ✔  2 PCE inventory       ││   10.52.144.143     Zabbix Proxy OCI 10.52…  NEW                           │
│ ✔  3 Labels              ││   192.168.161.105   DNS Corporativo 192.16…  CONFLICT-MULTIPLE   dns1      │
│ ▶  4 Reconcile by IP     ││   …                                                                        │
│ ○  5 Review new          ││ ■ Selected: 10.43.43.21                                                    │
│ ○  6 Dry run             ││ proposed                            in the PCE · unmanaged                 │
│ ○  7 Execute             ││   name      Zabbix Server 10.43.43.21   name      Zabbix Server            │
│ ○  8 Verify              ││   role      R_Monitoring                role      —                        │
│ ○  9 IP lists            ││   app       A_Observability             app       —                        │
│ ○ 10 Report              ││                                                                            │
│ EVENTS                   │╰────────────────────────────────────────────────────────────────────────────╯
│ 04:24 reconcile: 39 NEW… │╭─ workloader output · tab to scroll ────────────────────────────────────────╮
╰──────────────────────────╯│ 04:24:51 $ workloader wkld-export --output-file runs/…/pce-workloads.csv   │
 u update  s skip  c create anyway  r rename+update  U/S all of this kind  enter next  tab log  ? help  q quit
```

### Install

Prebuilt binaries are attached to the GitHub releases (`umwl-tui-darwin-arm64`, `umwl-tui-darwin-amd64`, `umwl-tui-linux-amd64`, `umwl-tui-linux-arm64`, plus `SHA256SUMS`). On a Mac, in the working folder:

```bash
gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui
```

Or build it (Go 1.24+): `(cd kit && make build) && cp kit/umwl-tui .`. `make dist` cross-compiles every platform into `dist/` with checksums; `make test` runs the headless TUI tests against `testdata/mock-workloader.py`.

### Use

```bash
./umwl-tui --setup-only                                   # step 0 only: workloader, pce.yaml (pce-add --api-key), connection test
./umwl-tui customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1
```

| Flag | Meaning |
|---|---|
| `<csv>` | CSV of proposed workloads, one row per IP (`wkld-import` format). Optional: without it the file chooser opens. |
| `--setup-only` | Only install/verify workloader and the PCE connection; loads nothing. |
| `--ipl <csv>` | IP list CSV (`ipl-import` format) for step 9. |
| `--pce <name>` | PCE name in `pce.yaml` (passed as `--pce` to workloader). Required if `pce.yaml` has more than one PCE. |
| `--priority 1` or `1,2` | Load only the rows whose `description` starts with `[.. P<n> ..]` for those priorities. |
| `--workloader <path>` | Path to the binary (default `./workloader`, then the PATH). |
| `--chunk 20` | Rows per `wkld-import` call: progress granularity and blast radius of an error. |
| `--runs ./runs` | Folder where `runs/<timestamp>/` is created. |

### The ten steps

| Step | What it does | Touches the PCE |
|---|---|---|
| 0 Preflight | Locates workloader (or downloads/builds it), shows the PCE (`pce-list`), configures it with `pce-add --api-key` if needed, tests the connection, creates `runs/<ts>/`. | Read-only |
| 1 Load CSV | Required columns (`hostname`, `name`, `interfaces`), valid IPs, repeated IPs, `--priority` filter, duplicate hostnames or names (appends the IP as a suffix), summary of values per label. | No |
| 2 PCE inventory | `wkld-export`, `label-export`, `label-dimension-export` into `runs/<ts>/`. Indexes every PCE workload by its IPs (`interfaces` and `public_ip`) and marks the managed ones (`managed` or `ven_href`). | Read-only |
| 3 Labels | CSV columns that are not a PCE label type → discard or exit to create the type. Lists the new values that will be created and asks for confirmation. | No |
| 4 Reconcile by IP | Each row is classified: **NEW** (the IP does not exist), **EXISTS-UNMANAGED** (the IP belongs to a UMWL: by default the labels/description of the existing one are updated, by `href`), **CONFLICT-MANAGED** (the IP belongs to a workload with a VEN: skipped by default), **CONFLICT-MULTIPLE** (several workloads share the IP). Per row: update, skip, create anyway, rename, or apply the decision to all of the same kind. | No |
| 5 Review new | Table of the new ones; accept all or review one by one editing name/hostname/description/labels. Writes `to-create.csv`, `to-update.csv`, `skipped.csv`. | No |
| 6 Dry run | `wkld-import to-create.csv --umwl --update=false --match name` and `wkld-import to-update.csv --match href`, without `--update-pce`; shows the relevant lines of the workloader log. A dry run that reports `cannot be blank` or `nothing to be done` is treated as failed. Asks for explicit confirmation before going on. | No |
| 7 Execute | Splits into batches of `--chunk` rows and runs the same commands with `--update-pce --no-prompt` per batch, with a progress bar; on error: retry, skip the batch or abort. | **Yes** |
| 8 Verify | New `wkld-export` and confirms that every created IP appears as a workload. | Read-only |
| 9 IP lists | With `--ipl`: `ipl-import` dry run, confirmation, `ipl-import … --update-pce --no-prompt`. | **Yes** |
| 10 Report | `runs/<ts>/report.md` and `report.json`: created, updated, skipped, failed batches, labels created, verification and every command executed with its exit code. | No |

### Keys, file chooser, dialogs, run folder

- **Global**: `tab` switches focus to the log pane (scroll with the arrows), `?` help, `q` quit (asks once work has started), `ctrl+c` quit. Tables: `↑↓ j k pgup pgdown g G`. **Reconcile (step 4)**: `u` update the existing object, `s` skip, `c` create anyway, `r` rename and update, `U`/`S` apply to every row of the same kind, `n` next undecided row. **Review (step 5)**: `e` edit name/hostname/description/labels, `s` skip. **Dialogs**: every write (step 7 and step 9) sits behind a red "Write to the PCE?" modal; a failed batch opens retry / skip / abort; `esc` cancels a form; modals capture every key while open.
- **File chooser**: opens after the preflight when no CSV was given (editable path on top, folders and CSVs below, `..` to go up, `tab` or `/` to type an exact path, `~` expands), then asks for the optional IP lists CSV and the priority filter. A CSV that fails validation shows the reason and reopens the chooser.
- **Run folder** `runs/<timestamp>/`: PCE inventories before and after, `to-create.csv`, `to-update.csv`, `skipped.csv`, one CSV and one workloader log per batch (`create-chunkNNN.*`, `update-chunkNNN.*`), the dry-run logs, `report.md` and `report.json` (including every workloader command with its exit code, the PCE name and the working folder). The API secret never reaches the logs or the report. Design and development notes: `docs/SPEC-umwl-tui.md` and `CLAUDE.md`.

## Exporting the flows from the PCE

The input of the analysis is the PCE traffic CSV. In 22.x–23.x consoles the view is called **Explorer**; in 24.x/25.x consoles the same data is in the **Traffic** view (table) under **Explore** in the left menu, next to **Map**. The guide (§4) shows the screens schematically; the mechanics:

1. **Time window**: at least 7 days including a weekend and, if they exist, backup and scanning windows.
2. **Filters**: in *Source* (consumer) and *Destination* (provider) include the workloads with a VEN (or their application label) on one side and leave the other side open, with "or" between both sides to capture inbound and outbound; in *Service*, ports, protocols and processes.
3. **Result limit**: the console shows up to 10,000 connections per page and 100,000 on screen; the downloaded CSV can reach 200,000 on a standalone PCE. **An export exactly at the limit is a truncated export** (the one we analyzed came with exactly 5,000 rows, 93 % of them one port scan). Raise the limit to the maximum, and if it still fills up split by application/workload group and shorter windows, and exclude the scanner source.
4. **Run** (asynchronous queries appear later under **Load Results**, top right), then **Export** → CSV, named with customer, scope and dates (`customer-a_group1_2026-08-17_2026-09-01.csv`). **Open the CSV as text** (editor, `head`, Python). If you open it in Excel, import it with the wizard marking the IP columns as **Text**: Excel turns `10.0.4.10` into a number or a date. The same goes for the date columns.

Alternatives to the manual export:

- **workloader** (`workloader traffic`, "Export traffic data"): `--start` / `--end` (`yyyy-mm-dd` or `yyyy-mm-ddTHH:mm:ss`, GMT; default 88 days back and tomorrow), `--max-results` (default 100,000, maximum 200,000), `--incl-src-file` / `--excl-src-file` / `--incl-dst-file` / `--excl-dst-file` (files with hrefs from `label-export`, `ipl-export`, `wkld-export`), `--incl-svc-file` / `--excl-svc-file`, `--excl-allowed` / `--excl-potentially-blocked` / `--excl-blocked`, `--output-file`. The `explorer` command is still registered as `legacy-explorer`; use `traffic`. Its columns are not identical to the console's; the analysis instructions normalize both.
- **Asynchronous REST API** (the one the console uses): `POST /api/v2/orgs/<org>/traffic_flows/async_queries` with `query_name`, `sources`, `destinations`, `services`, `policy_decisions`, `start_date`, `end_date`, `max_results` (limit 200,000); then `GET …/traffic_flows/async_queries/<uuid>` for the status and `GET …/traffic_flows_async/queries/<uuid>/download` for the result. The synchronous `traffic_flows/traffic_analysis_queries` endpoint is deprecated.

The analysis also wants the current inventory: `wkld-export`, `label-export`, `label-dimension-export` and, optionally, `ipl-export --output-file pce-iplists.csv` (quick start, step 5). Export columns (24.x/25.x, one column per label type; full header in `docs/export-columns.txt`): `Source IP`, `Source Name`, `Source Hostname`, `Source Enforcement`, `Source App/Env/Loc/Role` (and any additional type), `Source FQDN`, the same for `Destination *`, `Port`, `Protocol`, `Process`, `Username`, `Num Flows`, `Bytes In`, `Bytes Out`, `Connection State`, `Reported Policy Decision`, `Reported by`, `First Detected`, `Last Detected`. The label columns are what separate traffic between managed workloads from traffic to IPs without a VEN.

## Policy objects and naming convention

- **Unmanaged workloads (UMWL).** Network entities without a VEN that are registered in the PCE so that rules can be written about them; policy between a workload with a VEN and an unmanaged one is enforced by the rules on the side that has the VEN. **A peer is modeled as a UMWL when it is a specific server with an identifiable role** (DNS resolvers, internal NTP, Zabbix server and proxies, Splunk indexers, NetBackup master/media, databases, load balancer VIPs or front-ends, bastions, vulnerability scanners, SMTP relays, IBM MQ, OEM). **One per IP**, with labels; it appears in the ringfence rules like any other workload. In the console it is *Workloads → Add → Add Unmanaged Workload*.
- **IP lists.** Static collections of addresses, ranges and FQDNs, for **broad peers or peers that are not servers**: user VLANs, whole application subnets, cloud metadata (`169.254.169.254`, which in OCI is also the VCN resolver), public NTP, EDR SaaS consoles, ranges published by a provider. They also coexist with the UMWLs as a shortcut: an `IPL_CDLV_OracleDB` list lets you write the "app → Oracle 1521" rule today and migrate it to labels later.
- **Labels and label types.** The four default types are Role, Application, Environment and Location (RAEL); the PCE supports additional types. Illumio applies OR between values of the same type and AND between different types. **workloader creates label values that do not exist, but does not create types**: a CSV column that is not a PCE type is silently ignored by `wkld-import` (umwl-tui detects it in step 3 and offers to discard it or exit to create the type with `label-dimension-import`).
- **Services.** Port/protocol objects (`SVC_DNS`, `SVC_App_8080`). The report proposes them, but **the kit does not create them**; load them with `workloader svc-import` or from the console when the rules are written.

| Field | Convention | Example |
|---|---|---|
| Labels | Prefix by type: `R_<Role>`, `A_<App>`, `E_<Environment>`, `L_<Location>` | `R_DNS`, `A_CoreInfra`, `E_Prod`, `L_OCI` |
| `name` | `<descriptive role> <IP>`; it is what the console shows and what makes each row unique | `Zabbix Server 10.43.43.21` |
| `hostname` | **Empty**. The FQDNs in the exports may be pseudonymized; a fake hostname confuses more than it helps. The create pass matches by `name` (`--match name`). | |
| `interfaces` | `eth0:<ip>` (the console shows it as *eth0: 10.1.1.1*); several interfaces separated by `;` | `eth0:10.43.43.21` |
| `description` | Analysis comment with the prefix `[<group> P<priority> conf:<Alta\|Media\|Baja>]`; the priority is what `--priority` filters on | `[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP…` |
| `review` | Working column (PENDING, UPDATED, UNCHANGED; umwl-tui writes NEW, EXISTS-UNMANAGED…); workloader ignores it | |
| IP lists | `IPL_<Site>_<Use>`; description starts with `[<group>]` and ends with the rule usage | `IPL_CDLV_UserVLAN` |

## workloader CSV contract

### `wkld-import` (workloads)

Recognized headers (everything else is ignored): `href`, `hostname`, `name`, `interfaces`, `public_ip`, `distinguished_name`, `spn`, `enforcement`, `visibility`, `description`, `os_id`, `os_detail`, `data_center`, `external_data_set`, `external_data_reference` and **one column per label type** (`role`, `app`, `env`, `loc`, and any custom types that exist in the PCE).

```
hostname,name,interfaces,description,role,app,env,loc,review
,Zabbix Server 10.43.43.21,eth0:10.43.43.21,[C3 P1 conf:Alta] Servidor Zabbix…,R_Monitoring,A_Observability,E_Prod,L_CDLV,PENDING
```

- `interfaces`: `192.168.200.20`, `192.168.200.20/24`, `eth0:192.168.200.20` or `eth0:192.168.200.20/24`, separated by `;`.
- **Matching**: workloader matches on **one** column, chosen by `--match` (`href|hostname|name|external_data`) or, without the flag, by priority over the columns present: `href`, then `hostname`, then `name`. Rows whose match column is blank are skipped (`the match column cannot be blank`); there is no fallback from hostname to name. **Never by IP.** Hence the kit's flags — `--match name` on the create pass (blank hostnames) and `--match href` on the update pass — and the IP reconciliation: without it, a UMWL that already exists under another name is created as a duplicate.
- `--umwl`: creates unmanaged workloads when the host does not exist (disabled when matching by href). `--update` (default `true`): updates existing ones; `--update=false` only creates. `--allow-enforcement-changes`: required to touch `enforcement`/`visibility`. `--max-create` / `--max-update`: safety cap (-1 = no limit).
- Without `--update-pce`, the command is a **dry run**: it writes nothing and leaves in `workloader.log` (or `--log-file`) what it would create and change. With `--update-pce` it asks for confirmation; `--update-pce --no-prompt` does not (umwl-tui already asked).

### `ipl-import` (IP lists)

```
name,description,include,exclude,fqdns
IPL_CDLV_DNS,[C4] Corporate resolvers — uso: reglas DNS,192.168.161.92;192.168.161.104;192.168.161.105,,
```

- `include` / `exclude`: IPs, CIDRs or ranges separated by `;`. `fqdns` for names.
- Matching by `href` and, if absent, by `name`: if the name exists, it updates; if not, it creates. `--ignore-href` to reuse an export from another PCE. `--provision` (`-p`) provisions after creating/updating. Without `--update-pce` it is a dry run.

## Anonymization

Exports carry hostnames, FQDNs, usernames and sometimes the customer's name. `anonymize_export.py` replaces them consistently (same input → same token, across every column and every run that shares the map) and reversibly; private IPs, ports, protocols, processes, label values, counts and dates are kept because they are the evidence.
```bash
python3 kit/anonymize_export.py anon TrafficData.csv -o TrafficData.anon.csv --map anon-map.json \
    --customer "Customer A" --customer acme --domain customer-a.com --domain corp.acme.local [--public-ips]
python3 kit/anonymize_export.py deanon C4-umwl-import.csv -o customer-a-umwl-import.csv --map anon-map.json
```

`anon` maps the Source/Destination Name, Hostname and FQDN columns to `host-0001.company.com` style tokens (domains to `company.com` / `dept.company.com` so tiers stay recognizable), usernames to `user-01…` (well-known service accounts such as `root`, `oracle`, `zabbix` are kept), `--customer` names to "Cliente", and public IPs to `203.0.113.x` / `198.51.100.x` only with `--public-ips`; it ends with a leak check. `deanon` applies the inverse map to any CSV, so the proposed workloads carry the real names in their descriptions before loading. `anon-map.json` (mode 0600, gitignored) is the secret: it never leaves the working folder. The guide (§5) covers what to anonymize and what to attach.

## Analysis with Claude: Project instructions and prompt

The analysis is reproduced with a Claude Project whose instructions are `docs/prompts/context.md` and whose per-conversation message is `docs/prompts/prompt-short.md` (v1 and v2 variants); attach the pseudonymized export and the `pce-*.csv` inventory. Outside a Project, attach `context.md` and start with "Follow context.md". The prompt is not duplicated here (deliverables are in Spanish because that is how the kit is used with LATAM customers; change the language in `context.md` §0 if needed); in short it asks for:

1. Work only from the attached files; cite the flows behind every inference; verify external facts against official documentation; quality gate before answering (consistent counts, no hostname-derived roles, no invented ports, CSVs that parse with the exact headers and unique names).
2. Normalize first: Excel-mangled IPs, mixed date formats, truncation at the query limit, populations (VENs in scope, lab hosts, cloud flow logs, scanners), policy-decision vocabulary.
3. Classify every peer without a VEN: UMWL (one per IP, identifiable server role) or IP list (VLANs, subnets, cloud metadata, SaaS by FQDN); load balancers only for VIPs.
4. Label model `R_/A_/E_/L_`, reusing the values already in the PCE; `A_` defines the ringfence.
5. Security findings `S-xx` with severity, evidence and the Illumio policy action; stable IDs across versions.
6. Deliverables: self-contained HTML report in Spanish, customer-facing, without the customer's name, twelve sections with inline SVG diagrams; `<group>-umwl-import.csv` and `<group>-ipl-import.csv` in the exact workloader contracts above; optional XLSX workbook. For a v2: keep every entry and finding ID, mark "solo v1" / "nuevo", deliver the **full** CSVs again with `review` = PENDING / UPDATED / UNCHANGED.

After a v2 CSV, umwl-tui does the rest: in step 4 the rows whose IP already exists come out as EXISTS-UNMANAGED and are updated by `href`; the new ones are created.

## Without the TUI: equivalent workloader commands

Same working folder, same `pce.yaml`; `reconcile_umwl.py` (legacy) does the IP reconciliation without writing:

```bash
./workloader pce-add --api-key --name customer-a --fqdn pce.customer-a.example --port 443 \
    --api-user api_xxxxxxxx --api-secret '…' --org 1 --disable-tls-verification false
./workloader wkld-export --output-file pce-workloads.csv                                    # inventory (check pce-list first)
python3 kit/legacy/reconcile_umwl.py pce-workloads.csv customer-a-umwl-import.csv           # -to-create / -existing / -conflicts
./workloader wkld-import customer-a-umwl-import-to-create.csv --umwl --update=false --match name   # dry run -> workloader.log
./workloader wkld-import customer-a-umwl-import-existing.csv --match href                   # labels/description only, dry run
./workloader ipl-import customer-a-ipl-import.csv                                           # dry run
# same three commands with --update-pce to write (add --no-prompt to skip workloader's confirmation)
```

`reconcile_umwl.py` leaves next to the proposed CSV: `-to-create.csv` (IPs that do not exist), `-existing.csv` (IPs of existing UMWLs, with `href` and `hostname` rewritten so that `wkld-import --match href` only updates labels and description), `-conflicts.csv` (IPs of workloads with a VEN or shared: decide by hand) and `-reconcile-report.txt`.

## Legacy: plain-terminal loaders

`legacy/umwl_loader.py` (interactive, sequential prompts, same ten steps) and `legacy/reconcile_umwl.py` (non-interactive reconciliation) were the first implementation, in Python 3 with the standard library only. They are kept for environments where the Go binary cannot run and as a readable reference; they use the same workloader flags as umwl-tui (`pce-add --api-key`, `--match name` / `--match href`, the dry-run failure rule), but new features land in umwl-tui first. Run them from the working folder: `python3 kit/legacy/umwl_loader.py --setup-only`, then `python3 kit/legacy/umwl_loader.py customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1` (same flags as umwl-tui). Details in [legacy/README.md](legacy/README.md).

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| macOS does not let you run `umwl-tui` or `workloader` ("cannot be opened because the developer cannot be verified") | Gatekeeper quarantine on the download. `xattr -d com.apple.quarantine ./umwl-tui` (umwl-tui does it for workloader when it downloads it). |
| `workloader` on Apple Silicon | The macOS release is an Intel binary and runs through Rosetta 2 (`softwareupdate --install-rosetta` if it is not installed). For a native binary: `brew install go`; `umwl-tui --setup-only` offers to clone and build. |
| Dry run ends with `the match column cannot be blank` on every row and `nothing to be done` | workloader matches on one column chosen by `--match` or, without it, by priority over the columns present (href, hostname, name); with the blank-hostname convention it picks `hostname` and skips every row. umwl-tui and the legacy loader pass `--match name` on the create pass and `--match href` on the update pass, and treat that dry run as failed. By hand: `wkld-import to-create.csv --umwl --update=false --match name`. |
| `pce-add` asks for email and password even though you passed `--api-user` | The `--api-key` flag is missing; without it workloader ignores `--api-user/--api-secret/--org`. umwl-tui always passes it. |
| 401/403 when importing; the dry run works | The API key inherits the user's role. Create it with a Global Organization Owner or Global Administrator user (a scoped Workload Manager can create workloads, but not new labels or IP lists), or use a service account with those permissions. Check the `org id` too. |
| TLS error with an on-prem PCE and an internal certificate | `pce-add … --disable-tls-verification true` only in a lab; in production install the CA in the keychain. |
| The PCE shown by `pce-list` is not the customer's (or is another organization) | Wrong folder (the current directory decides which `pce.yaml` is used) or `ILLUMIO_CONFIG` points to another file. One folder per account + PCE; `unset ILLUMIO_CONFIG`; `--pce <name>` only if the `pce.yaml` has several profiles. |
| The dry run updates a workload you did not expect, or two rows load as one | Match by `name` with an existing object, or two rows with the same match value. Check `runs/<ts>/dry-*.log`; rename the row (step 4/5) or let the IP reconciliation resolve it; umwl-tui appends the IP as a suffix to duplicates, by hand make the `name` unique. |
| A label column was not applied | The label type does not exist in the PCE; `wkld-import` ignores the column. Create it in the console (Settings → Label Settings) or with `./workloader label-dimension-import types.csv --update-pce` (CSV with `key,display_name`), and run again. |
| `wkld-import` creates the label with different capitalization | Values are case-sensitive unless `--ignore-case`. Use exactly the values shown by `label-export`. |
| Failed batch in step 7 | Retry / skip / abort in the modal. Afterwards check `runs/<ts>/create-chunkNNN.log`; the CSV of that batch stays in `create-chunkNNN.csv` to retry by hand with `wkld-import … --umwl --update=false --match name --update-pce`. |
| The proposed CSV still carries `host-0012.company.com` names | The `deanon` pass was skipped, or run with a different `anon-map.json`. Rerun it with the map of the folder where `anon` ran. |

## Guides (PDF/HTML)

One merged guide (30 pages, same content in both languages) explains with diagrams the workflow at a glance, the requirements and the one-folder-per-account + PCE rule, the initialization, exporting from the PCE with schematic screens, anonymization, the analysis with Claude, the policy objects, umwl-tui step by step with its decision points, umwl-tui and the workloader subcommands, the v2 cycle, troubleshooting and a checklist, and the export columns and repository files as appendices.

- English: [docs/Guide-umwl-tui-EN.pdf](docs/Guide-umwl-tui-EN.pdf) (self-contained HTML: [docs/Guide-umwl-tui-EN.html](docs/Guide-umwl-tui-EN.html)).
- Español: [docs/Guia-umwl-tui-ES.pdf](docs/Guia-umwl-tui-ES.pdf) (HTML autocontenido: [docs/Guia-umwl-tui-ES.html](docs/Guia-umwl-tui-ES.html)).

## Sources consulted

- workloader (brian1917): README, releases v12.1.9, `cmd/root.go`, `cmd/wkldimport/cmd.go`, `cmd/iplimport/iplimport.go`, `cmd/wkldexport/headers.go`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension/import.go`, `cmd/traffic/traffic.go` — https://github.com/brian1917/workloader
- Illumio Core 25.4 — Visualization — About the Visualization Tools (Explore, Traffic, limits, asynchronous queries) — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.4/Visualization/out/en/visualization-tools/about-the-visualization-tools.html
- Illumio Core 25.1 — Visualization — Traffic Table (filters, Export) — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.1/Visualization/out/en/visualization-tools/traffic-table.html
- Illumio Core 22.5 — REST API — Explorer (async_queries, max_results 200,000) — https://product-docs-repo.illumio.com/Tech-Docs/Core/22.5/REST-APIs/out/en/core-22-5-rest-api-developer-guide/visualization/explorer.html
- Illumio Core 24.2 — REST API — API Keys (My API Keys, permissions) — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/REST-APIs/out/en/rest-apis/authentication-and-api-user-permissions/api-keys.html
- Illumio Core 24.5 — Security Policy — Workload Setup Using PCE Web Console (Add Unmanaged Workload) — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.5/Security-Policy/out/en/security-policy-guide-24-5/workloads/workload-setup-using-pce-web-console.html
- Illumio Core 24.2 — Getting Started — Policy Objects — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/Getting%20Started/out/en/policy-overview/policy-objects.html
- Illumio Core 25.4 — Security Policy — Create a Label Type — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.4/Security-Policy/out/en/security-policy-objects/about-labels-and-label-groups/label-types/create-a-label-type.html

## License

MIT. See [LICENSE](LICENSE).
