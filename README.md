Español: [README.es.md](README.es.md)

# illumio-workloader-import-kit

Kit to load into an Illumio PCE the **unmanaged workloads (UMWL)** and **IP lists** that come out of an Explorer flow analysis, using [workloader](https://github.com/brian1917/workloader) as the import engine. It includes `umwl-tui`, a full-screen terminal application (Go) that reconciles by IP against the PCE inventory before writing anything, its plain-terminal Python predecessor, a minimal non-interactive script, the sample CSVs from a proof of concept and an explanatory PDF guide with diagrams (in Spanish and in English).

> Verified against **workloader v12.1.9** (release of June 12, 2025; code on `master` consulted on September 3, 2026) and **Illumio Core 24.x/25.x** (documentation at `product-docs-repo.illumio.com`). The command names, flags and CSV headers in this README were taken from the workloader source code (`cmd/root.go`, `cmd/wkldimport`, `cmd/iplimport`, `cmd/wkldexport`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension`, `cmd/traffic`). If you use another version, confirm with `./workloader <command> --help`.

---

## Important: one working folder per Illumio account (organization) + PCE

workloader resolves the PCE connection by reading, in this order, `--config-file`, the `ILLUMIO_CONFIG` variable and, if neither is set, **`./pce.yaml` in the working folder**. The kit also writes `runs/<timestamp>/` in the working folder, with the inventory exported from the PCE, the final CSVs and the logs of every batch.

The isolation unit is neither "the customer" nor "the PCE" but each **Illumio account or organization + PCE** combination, that is, each distinct `pce.yaml` profile (FQDN, port, org id and API key):

- An account can have several PCEs (SaaS and on-prem, production and DR, regions): each one is a folder.
- A SaaS PCE (one FQDN) hosts several organizations; each `org id` is a separate tenant and gets its own folder, even when the FQDN is the same. The `org id` is the number that appears in the console URL (`…/orgs/<id>/…`).
- Another account: same layout, never the same `pce.yaml`.

If the `pce.yaml` files of two tenants share a folder (or a `pce.yaml` holds several PCEs and you forget `--pce`), the risk is concrete: **importing one customer's objects into another customer's PCE or organization**. The rule is simple: **one working folder per account + PCE**, with its own `pce.yaml`, its CSVs and its `runs/`. Suggested naming: `~/illumio/<account>/<pce-or-org>/`.

Suggested layout:

```
~/illumio/
├── customer-a/
│   ├── scp57-org12/                  # account customer-a, SaaS PCE scp57, org 12
│   │   ├── kit/                      # clone of this repository
│   │   ├── workloader                # binary (or a symlink to a shared one)
│   │   ├── pce.yaml                  # ONLY this account + PCE
│   │   ├── customer-a-umwl-import.csv
│   │   ├── customer-a-ipl-import.csv
│   │   └── runs/
│   │       └── 20260903-101500/
│   └── onprem-prod/                  # same account, another PCE: another folder, another pce.yaml
│       ├── kit/  workloader  pce.yaml  ...
└── customer-b/
    └── <pce-or-org>/                 # another account, same layout
        ├── kit/  workloader  pce.yaml  ...
```

Before every run, the TUI executes `workloader pce-list` and shows you the PCE it is going to use; read that name and the FQDN before accepting. If the `pce.yaml` has more than one PCE, always pass `--pce <name>`. workloader supports several profiles in a single `pce.yaml` (with `default_pce_name` and the `--pce` flag), but the kit deliberately does not rely on that: a forgotten `--pce` or a wrong default would load the objects into the wrong tenant.

---

## Repository contents

| File | What it is |
|---|---|
| `umwl-tui` (`cmd/`, `internal/`, `Makefile`) | **The full-screen application (Go, Bubble Tea)**: status bar, step list, per-step panel, live workloader output and key bar; modals for every decision; batches with progress and retry/skip; report per run. Single static binary for macOS (arm64/amd64) and Linux. See the section below and `docs/SPEC-umwl-tui.md`. |
| `umwl_loader.py` | Plain-terminal version (Python 3, standard library only) of the same flow, kept as fallback. Installs or verifies workloader, configures the PCE, validates the CSV, reconciles by IP, runs a dry run, imports in batches with a progress bar, verifies and leaves a report. |
| `reconcile_umwl.py` | Minimal, non-interactive version of the IP reconciliation step: from a `wkld-export` and the proposed CSV it generates `-to-create.csv`, `-existing.csv`, `-conflicts.csv` and a text report. It does not write to the PCE. |
| `examples/*-umwl-import.csv` | Proposed unmanaged workload CSVs (one row per IP) for three groups of a proof of concept. They are **POC examples**: they contain the customer's lab IPs and serve as a format template, not for loading as-is. |
| `examples/cliente3-umwl-import-v2.csv` | **Recommended format** from now on: labels with the `R_/A_/E_/L_` prefix, `name` = descriptive role + IP, empty `hostname`, `interfaces` = `eth0:<ip>`, report comment in `description`. |
| `examples/*-ipl-import.csv` | Proposed IP lists in the `workloader ipl-import` format. `cliente3-ipl-import-v2.csv` uses the `IPL_<Site>_<Use>` convention. |
| `docs/Guia-workloader-import-kit.pdf` | Explanatory guide in Spanish with diagrams (end-to-end flow, requirements and one folder per account + PCE, initialization, TUI steps, kit/workloader relationship, CSV → PCE object mapping, troubleshooting). Also as `docs/Guia-workloader-import-kit.html` (single file, opens with a double click). |
| `docs/Guide-workloader-import-kit-EN.pdf` | The same guide in English. Also as `docs/Guide-workloader-import-kit-EN.html`. |
| `.gitignore` | Excludes `workloader` (binary), `workloader/` (clone), `pce.yaml`, `*.log`, `runs/`, zips. **Never push `pce.yaml`: it contains the API key.** |

Changes from the original version of the kit:

- In `umwl_loader.py` both PCE setup paths (`[k]` enter the values in the TUI, `[i]` run workloader's own prompts) call `pce-add --api-key`, so workloader asks for API Authentication Username / API Secret / Org and never for email and password. Without that flag, workloader ignores `--api-user/--api-secret/--org` and asks for email and password (verified in `cmd/pcemgmt/addpce.go`).
- The TUI detects the "no pce configured" message that `workloader pce-list` prints (with exit code 0) when `pce.yaml` exists but has no entries, and goes straight to `pce-add` instead of asking whether that is the right PCE.

---

## Requirements

- **macOS** (tested on Apple Silicon; the flow is the same on Intel and on Linux).
- **Python 3.9 or later**, standard library only (there is no `pip install`).
- **workloader v12.x**. No installation required: it is a binary. The macOS release is published as `mac-<version>.zip`; on Apple Silicon it runs through Rosetta 2. If you prefer a native binary, **Go** (`brew install go`) and `go build` from the repository clone; the TUI offers both options.
- A **PCE API key with write permissions**. The key inherits the role of the user who creates it: if the user is read-only, `wkld-import --update-pce` fails. To create it: user menu (top right) → **My API Keys** → **Add**; save the **Authentication Username** (`api_…`) and the **Secret**, which are shown only once. A service account with equivalent permissions also works.
- **Network access to the PCE over HTTPS** (port 443 on SaaS, typically 8443 on-prem) from the Mac. On SaaS the `org id` is the number that appears in the console URL (`…/orgs/<id>/…`).

---

## Step by step: extracting the flows from Explorer

The input of the analysis is the PCE traffic CSV. The name of the view changed between console generations: in 22.x–23.x consoles it is called **Explorer**; in 24.x/25.x consoles the same data is in the **Traffic** view (table) under the **Explore** category of the left menu, next to **Map**. The mechanics are the same:

1. Open the PCE console and go to the **Explorer / Traffic** view.
2. **Time window**: pick a range (last day, week, month or a custom range). To infer roles with confidence, a window of at least 7 days that includes a weekend and, if they exist, backup and scanning windows is advisable.
3. **Filters**: in *Source* (consumer) and *Destination* (provider) you can include or exclude workloads, IPs and labels; in *Service*, ports, protocols and processes. For a proof of concept the usual approach is to include in Source **or** Destination the workloads with a VEN (or their application label) and leave the other side open, with the "or" operator between both sides to capture inbound and outbound.
4. **Result limit**: the maximum number of rows in the export is set by the query. The console shows up to 10,000 connections per page and up to 100,000 results on screen; the downloaded CSV can reach 200,000 on a standalone PCE. **The export we analyzed came with exactly 5,000 rows**, which was the limit configured in that query, and a single port scan took up 93 % of them. Before exporting, raise the limit to the maximum the console allows and, if it still fills up, **split the query by application/workload group and by shorter time windows**, and exclude the scanner source. An export exactly at the limit is a truncated export.
5. Run the query (**Run**). If the query is asynchronous, it appears later under **Load Results** (top right); from there it can be opened and exported.
6. **Export** → CSV. Save the file with a name that includes customer, scope and dates (for example `customer-a_group1_2026-08-17_2026-09-01.csv`).
7. **Open the CSV as text** (editor, `head`, Python, pandas). If you open it in Excel, import it with the wizard marking the IP columns as **Text**: Excel interprets `10.0.4.10` as a number or a date and rebuilding the IPs afterwards is wasted work. The same goes for the date columns: in previous exports two formats arrived mixed.

Export columns (24.x/25.x console; one column per label type): `Source IP`, `Source Name`, `Source Hostname`, `Source Enforcement`, `Source App/Env/Loc/Role` (and any additional type), `Source FQDN`, the same for `Destination *`, `Port`, `Protocol`, `Process`, `Username`, `Num Flows`, `Bytes In`, `Bytes Out`, `Connection State`, `Reported Policy Decision`, `Reported by`, `First Detected`, `Last Detected`. The source and destination label columns are what allow separating traffic between managed workloads from traffic to IPs without a VEN.

### Alternatives to the manual export

- **Asynchronous REST API** (the same one the console uses): `POST /api/v2/orgs/<org>/traffic_flows/async_queries` with `query_name`, `sources`, `destinations`, `services`, `policy_decisions`, `start_date`, `end_date` and `max_results` (limit 200,000); then `GET …/traffic_flows/async_queries/<uuid>` for the status and `GET …/traffic_flows_async/queries/<uuid>/download` for the result. The synchronous endpoint `traffic_flows/traffic_analysis_queries` is deprecated.
- **workloader**: the current command is `workloader traffic` ("Export traffic data"). Relevant flags: `--start` / `--end` (`yyyy-mm-dd` or `yyyy-mm-ddTHH:mm:ss`, in GMT; default 88 days back and tomorrow), `--max-results` (default 100,000, maximum 200,000), `--incl-src-file` / `--excl-src-file` / `--incl-dst-file` / `--excl-dst-file` (files with hrefs of labels, IP lists or workloads, obtained with `label-export`, `ipl-export`, `wkld-export`), `--incl-svc-file` / `--excl-svc-file`, `--excl-allowed` / `--excl-potentially-blocked` / `--excl-blocked`, `--output-file`. The `explorer` command is still registered but as the `legacy-explorer` package; use `traffic`.

  ```bash
  ./workloader traffic --start 2026-08-17 --end 2026-09-01 --max-results 200000 --output-file flows.csv
  ```

  The columns of the `workloader traffic` CSV are not identical to those of the console export; the analysis prompt (below) asks to normalize column names, so either one works.

---

## Policy objects: what the kit creates and why

- **Unmanaged workloads (UMWL).** Network entities without a VEN that are registered in the PCE so that rules can be written about them; policy between a workload with a VEN and an unmanaged one is enforced by the rules on the side that has the VEN. The analysis criterion: **a peer is modeled as a UMWL when it is a specific server with an identifiable role** (DNS resolvers, internal NTP, Zabbix server and proxies, Splunk indexers, NetBackup master/media, databases, load balancers or front-ends, bastions, vulnerability scanners, SMTP relays, IBM MQ, OEM). **One per IP** is created, with labels, and it appears in the ringfence rules like any other workload. In the console it is the equivalent of *Workloads → Add → Add Unmanaged Workload*.
- **IP lists.** Static collections of addresses, ranges and FQDNs. They are used for **broad peers or peers that are not servers**: user VLANs, whole application subnets, cloud metadata (`169.254.169.254`, which in OCI is also the VCN resolver), public NTP, EDR SaaS consoles, ranges published by a provider. They also coexist with the UMWLs as a shortcut: an `ipl-oracle-db` list lets you write the "app → Oracle 1521" rule today and migrate it to labels later.
- **Labels and label types.** The four default types are Role, Application, Environment and Location (RAEL); the PCE supports additional types (for example `os`). When writing rules, Illumio applies OR between values of the same type and AND between different types. **workloader creates label values that do not exist, but does not create types**: if the CSV brings a column that is not a PCE type, `wkld-import` silently ignores it (the TUI detects it in step 3 and offers to discard it or exit to create the type with `label-dimension-import`).
- **Services.** Port/protocol objects (`svc-dns`, `svc-app-8080`). The report proposes them, but **the kit does not create them**; they are loaded with `workloader svc-import` or from the console when the rules are written.

### Naming convention

| Field | Convention | Example |
|---|---|---|
| Labels | Prefix by type: `R_<Role>`, `A_<App>`, `E_<Environment>`, `L_<Location>` | `R_DNS`, `A_CoreInfra`, `E_Prod`, `L_OCI` |
| `name` | `<descriptive role> <IP>`; it is what the console shows and what makes each row unique | `Zabbix Server 10.43.43.21` |
| `hostname` | **Empty**. The FQDNs in the exports may be anonymized or invented; a fake hostname confuses more than it helps. workloader identifies the row by `name` when there is no hostname. | |
| `interfaces` | `eth0:<ip>` (the console shows it as *eth0: 10.1.1.1*); several interfaces separated by `;` | `eth0:10.43.43.21` |
| `description` | Analysis comment with the prefix `[<group> P<priority> conf:<Alta\|Media\|Baja>]`; the priority is what `--priority` filters on | `[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP…` |
| `review` | Working column (PENDING, NEW, EXISTS-UNMANAGED…); workloader ignores it | |

---

## workloader CSV contract

### `wkld-import` (workloads)

Recognized headers (everything else is ignored): `href`, `hostname`, `name`, `interfaces`, `public_ip`, `distinguished_name`, `spn`, `enforcement`, `visibility`, `description`, `os_id`, `os_detail`, `data_center`, `external_data_set`, `external_data_reference` and **one column per label type** (`role`, `app`, `env`, `loc`, and any custom types that exist in the PCE).

```
hostname,name,interfaces,description,role,app,env,loc,review
,Zabbix Server 10.43.43.21,eth0:10.43.43.21,[C3 P1 conf:Alta] Servidor Zabbix…,R_Monitoring,A_Observability,E_Prod,L_CDLV,PENDING
```

- `interfaces`: `192.168.200.20`, `192.168.200.20/24`, `eth0:192.168.200.20` or `eth0:192.168.200.20/24`, separated by `;`.
- **Matching**: workloader looks for the existing workload by `href` if present, else by `hostname`, else by `name` (`--match` allows forcing `href|hostname|name|external_data`). **Never by IP.** That is why two rows with the same `name` and no hostname are treated as the same workload (the TUI appends the IP as a suffix), and why the IP reconciliation exists: without it, a UMWL that already exists under another name is created as a duplicate.
- `--umwl`: creates unmanaged workloads when the host does not exist (disabled when matching by href). `--update` (default `true`): updates existing ones; `--update=false` only creates. `--allow-enforcement-changes`: required to touch `enforcement`/`visibility`. `--max-create` / `--max-update`: safety cap (-1 = no limit).
- Without `--update-pce`, the command is a **dry run**: it writes nothing and leaves in `workloader.log` (or in `--log-file`) what it would create and what it would change. With `--update-pce` it asks for confirmation; with `--update-pce --no-prompt` it does not (automation).

### `ipl-import` (IP lists)

```
name,description,include,exclude,fqdns
ipl-dns-corp,Corporate resolvers,192.168.161.92;192.168.161.104;192.168.161.105,,
ipl-oci-metadata,OCI VCN resolver and metadata,169.254.169.254/32,,
```

- `include` / `exclude`: IPs, CIDRs or ranges separated by `;`. `fqdns` for names.
- Matching by `href` and, if absent, by `name`: if the name exists, it updates; if not, it creates. `--ignore-href` to reuse an export from another PCE. `--provision` (`-p`) provisions after creating/updating.
- Same as above: without `--update-pce` it is a dry run.

---

## Initializing the working folder

These steps are done once per account + PCE combination. They leave a folder with the kit clone, the workloader binary and a tested `pce.yaml`; after that, every load is a single command. The kit repository is private: a GitHub session (`gh auth login` or an SSH key loaded into the account) is needed before cloning.

### a. Working folder for this account + PCE

```bash
mkdir -p ~/illumio/customer-a/scp57-org12 && cd ~/illumio/customer-a/scp57-org12
```

The folder is the isolation unit; everything that follows runs with that folder as the current directory.

### b. The kit inside the folder (private repository)

```bash
gh auth login                                   # once; or an SSH key loaded into GitHub
git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit
# equivalent: gh repo clone roschereric/illumio-workloader-import-kit kit
# without git: on GitHub, Code → Download ZIP, and unpack it as kit/
```

Expected result. The scripts run from the working folder as `python3 kit/umwl_loader.py …`: the current directory is the working folder, so `./workloader`, `./pce.yaml` and `./runs` sit next to the CSVs and not inside the clone. The CSVs live in the working folder, not in `kit/`; `kit/examples/` contains templates only. Running inside the clone itself also works (its `.gitignore` excludes `pce.yaml`, the binary and `runs/`), at the cost of tying that clone to a single account + PCE.

```
~/illumio/customer-a/scp57-org12/
  kit/                      # repository clone; update with: git -C kit pull
  workloader                # binary; the TUI downloads it (or a symlink to a shared one)
  pce.yaml                  # ONLY this account + PCE; created by pce-add; never copy it
  customer-a-umwl-import.csv
  customer-a-ipl-import.csv
  runs/                     # one subfolder per run
```

### c. workloader and pce.yaml with the TUI

(With the Go application: `./umwl-tui --setup-only` does the same step 0 — see the umwl-tui section.)

```bash
python3 kit/umwl_loader.py --setup-only
```

The TUI looks for `./workloader` (then the PATH, or the `--workloader` path). If it does not find it, it offers to download the release (`mac-<version>.zip`, with `curl` + `unzip` and `xattr -d com.apple.quarantine`) or to clone the workloader repository into `workloader-src/` and build it with `go build`. Then it runs `pce-list`; if there is no `pce.yaml`, or workloader replies "no pce configured", it goes straight to `pce-add`: it asks for a short name, FQDN, port, API user, API secret and org id, runs `pce-add --api-key …` with those values, shows `pce-list` again and tests the connection with a `label-dimension-export`.

### d. Sanity check (read-only)

```bash
./workloader pce-list
./workloader wkld-export --output-file sanity.csv
# look at the hostnames in sanity.csv: is this the inventory of the expected tenant?
```

Before writing anything, a read-only export proves that the API key works and that the inventory belongs to the expected tenant.

### e. Reviewed CSVs and first load

```bash
cp ~/Downloads/customer-a-umwl-import.csv ~/Downloads/customer-a-ipl-import.csv .
python3 kit/umwl_loader.py customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1
```

The reviewed CSVs from the report go into the working folder, named after the customer. The TUI walks through steps 0 to 10 (see "How to use it"); it writes to the PCE only after the dry-run confirmation.

| Situation | What to do |
|---|---|
| f. Another account or another PCE | Repeat a–d in a new folder. Never copy `pce.yaml`. If you prefer a single kit clone, share it through a symlink: `ln -s ~/illumio/kit-shared kit`. |
| g. Updating the kit | `git -C kit pull` (or download the zip again). The loader has no other dependencies: there is nothing else to install. |
| Several PCEs in the same `pce.yaml` | Not the kit's scheme. If it exists anyway, always pass `--pce <name>` to `umwl_loader.py`; the TUI shows `pce-list` and asks whether it is the right PCE before going on. |

---

## umwl-tui: the full-screen application

`umwl-tui` is the same ten-step flow as `umwl_loader.py`, rebuilt as a real terminal application (htop / Midnight Commander style): a status bar with the PCE, the run folder and the step counter; the list of steps with their state on the left; the active step's panel on the right (tables you browse with the arrow keys, side-by-side "proposed vs. in the PCE" detail, progress bars); the live workloader output at the bottom; and a key bar that changes per step. Every decision that matters (conflicts, writes to the PCE, failed batches) is a modal; nothing is written before the "Write to the PCE?" confirmation after the dry run.

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

Prebuilt binaries are attached to the GitHub releases (`umwl-tui-darwin-arm64`, `umwl-tui-darwin-amd64`, `umwl-tui-linux-amd64`, `umwl-tui-linux-arm64`, plus `SHA256SUMS`). On a Mac:

```bash
cd ~/illumio/<account>/<pce>                       # the working folder (see "Initializing the working folder")
gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui
```

Or build it (Go 1.24+): `git clone … kit && (cd kit && make build) && cp kit/umwl-tui .`. `make dist` cross-compiles every platform into `dist/` with checksums.

### Use

```bash
./umwl-tui --setup-only                                   # step 0 only: workloader, pce.yaml (pce-add --api-key), connection test
./umwl-tui customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1
./umwl-tui --help                                         # --pce, --workloader, --chunk (default 20), --runs (default ./runs)
```

Keys: `tab` switches focus to the log pane (scroll with the arrows), `?` help, `q` quit (asks once work has started). In the reconcile step: `u` update the existing object, `s` skip, `c` create anyway, `r` rename and update, `U`/`S` apply to every row of the same kind, `n` next undecided row. In the review step: `e` edit name/hostname/description/labels, `s` skip. A failed batch opens a modal with retry / skip / abort.

Everything the run produced is in `runs/<timestamp>/`: PCE inventories before and after, `to-create.csv`, `to-update.csv`, `skipped.csv`, one CSV and one workloader log per batch, `report.md` and `report.json` (including every workloader command with its exit code).

Design and development notes: `docs/SPEC-umwl-tui.md` (state machine, UI contract, engine/workloader contract, security requirements and tests, backlog) and `CLAUDE.md` (build/test/release commands and conventions for Claude Code sessions).

## How to use it

### 1. Prepare the working folder

Always from the working folder of the account + PCE (see "Initializing the working folder"):

```bash
cd ~/illumio/customer-a/scp57-org12
# copy here the CSVs proposed for THIS tenant (not inside kit/)
python3 kit/umwl_loader.py --setup-only
```

`--setup-only` looks for `./workloader` (or `--workloader <path>`, or the PATH). If it does not find it, it offers to download the latest release (`curl` + `unzip`, removes the Gatekeeper quarantine) or to clone and build with Go. Then it runs `pce-list`; if there is no `pce.yaml` (or workloader replies "no pce configured") it guides the `pce-add` with the API key and tests the connection with a `label-dimension-export`.

### 2. Load

```bash
python3 kit/umwl_loader.py customer-a-umwl-import.csv --ipl customer-a-ipl-import.csv --priority 1
```

`umwl_loader.py` flags:

| Flag | Meaning |
|---|---|
| `csv` | CSV of proposed workloads, one row per IP (`wkld-import` format). |
| `--setup-only` | Only install/verify workloader and the PCE connection; loads nothing. |
| `--ipl <csv>` | IP list CSV (`ipl-import` format) for step 9. |
| `--pce <name>` | PCE name in `pce.yaml` (passed as `--pce` to workloader). Required if `pce.yaml` has more than one PCE (not the recommended scheme: one folder per account + PCE). |
| `--priority 1` or `1,2` | Load only the rows whose `description` starts with `[.. P<n> ..]` for those priorities. |
| `--workloader <path>` | Path to the binary (default `./workloader`, then the PATH). |
| `--chunk 20` | Rows per `wkld-import` call. Progress granularity and blast radius of an error. |
| `--runs ./runs` | Folder where `runs/<timestamp>/` is created. |

What each TUI step does:

| Step | What it does | Touches the PCE |
|---|---|---|
| 0 Preflight | Locates workloader, shows the PCE (`pce-list`), creates `runs/<ts>/`. | No |
| 1 Validate CSV | Required columns (`hostname`, `name`, `interfaces`), valid IPs, repeated IPs, `--priority` filter, duplicate hostnames or names (appends the IP as a suffix), summary of values per label. | No |
| 2 Inventory | `wkld-export`, `label-export`, `label-dimension-export` into `runs/<ts>/`. Indexes every PCE workload by its IPs (`interfaces` and `public_ip`) and marks the managed ones (`managed` or `ven_href`). | Read-only |
| 3 Labels | CSV columns that are not a PCE label type → discard or exit to create the type. Lists the new values that will be created and asks for confirmation. | No |
| 4 Reconcile by IP | Each row is classified: **NEW** (the IP does not exist), **EXISTS-UNMANAGED** (the IP belongs to a UMWL: by default the labels/description of the existing one are updated, by `href`), **CONFLICT-MANAGED** (the IP belongs to a workload with a VEN: skipped by default), **CONFLICT-MULTIPLE** (several workloads share the IP). For each existing/conflict: update, skip, create anyway, rename, or apply the decision to all of the same kind. | No |
| 5 Review NEW | Table of the new ones; accept all or review one by one editing name/hostname/description/labels. Writes `to-create.csv`, `to-update.csv`, `skipped.csv`. | No |
| 6 Dry run | `wkld-import to-create.csv --umwl --update=false` and `wkld-import to-update.csv`, without `--update-pce`; shows the relevant lines of the workloader log. Asks for explicit confirmation before going on. | No |
| 7 Run | Splits into batches of `--chunk` rows and runs `wkld-import … --update-pce --no-prompt` per batch with a progress bar; on error: retry, skip the batch or abort. | **Yes** |
| 8 Verify | New `wkld-export` and confirms that every created IP appears as a workload. | Read-only |
| 9 IP lists | With `--ipl`: `ipl-import` dry run, confirmation, `ipl-import … --update-pce --no-prompt`. | **Yes** |
| 10 Report | `runs/<ts>/report.md` and `report.json`: created, updated, skipped, failed batches, labels created, verification and every command executed with its exit code. | No |

### 3. Without the TUI (equivalent workloader commands)

Same working folder, same `pce.yaml`:

```bash
./workloader pce-add --api-key --name customer-a --fqdn pce.customer-a.example --port 443 \
    --api-user api_xxxxxxxx --api-secret '…' --org 1 --disable-tls-verification false
./workloader pce-list
./workloader wkld-export --output-file pce-workloads.csv           # inventory
python3 kit/reconcile_umwl.py pce-workloads.csv customer-a-umwl-import.csv
./workloader wkld-import customer-a-umwl-import-to-create.csv --umwl  # dry run -> workloader.log
./workloader wkld-import customer-a-umwl-import-to-create.csv --umwl --update-pce
./workloader wkld-import customer-a-umwl-import-existing.csv          # labels/description only, match by href
./workloader ipl-import customer-a-ipl-import.csv                      # dry run
./workloader ipl-import customer-a-ipl-import.csv --update-pce
```

`reconcile_umwl.py` leaves next to the proposed CSV: `-to-create.csv` (IPs that do not exist), `-existing.csv` (IPs of existing UMWLs, with `href` and `hostname` rewritten so that `wkld-import` without `--umwl` only updates labels and description), `-conflicts.csv` (IPs of workloads with a VEN or shared: decide by hand) and `-reconcile-report.txt`.

---

## Prompt to replicate the analysis

This prompt reproduces the complete work (flow analysis, Illumio-branded report and CSVs in the kit's naming convention) from an Explorer/Traffic export. Paste it into Claude with the export attached and, if you have them, screenshots of the existing labels in the PCE.

Note: the deliverables the prompt asks for are in Spanish and customer-facing, because that is how this kit is used with LATAM customers. If you need them in another language, change the language in the "Inputs" and "Deliverables" sections.

````text
You are an Illumio pre-sales engineer. I am attaching a PCE traffic export (Explorer / Traffic view) from a microsegmentation proof of concept and I need you to reproduce a complete analysis and its deliverables. Work in Spanish, with direct technical language and no filler adjectives.

## Inputs
- Explorer/Traffic export as CSV or XLSX (attached). It may come from the PCE console or from `workloader traffic`; normalize the column names to a common schema: src_ip, src_name, src_hostname, src_enforcement, src_labels (one per type), src_fqdn, the equivalent dst_*, port, proto, process, username, num_flows, bytes_in, bytes_out, connection_state, policy_decision, reported_by, first_detected, last_detected.
- Optional: screenshots of the PCE label and label type list, and of the list of workloads with a VEN. If I attach them, use exactly those label names; if not, propose new values with the naming convention below.
- Context I give you in the message: POC scope (which hosts have a VEN, which groups), short group name for the priority prefix (for example G1, G2, C3) and any known network data (user VLANs, cloud ranges, sites).

## Normalization (do it before analyzing and document what you found)
1. IPs mangled by Excel: numeric cells, dates or scientific notation where an IP should be (for example 10.0.4.10 turned into 10.004 or into a date). Rebuild them when unambiguous and flag the ones that are not; never invent octets.
2. Dates in mixed formats (dd/mm/yyyy, mm/dd/yyyy, ISO, with and without time): unify to ISO 8601 and explain the disambiguation criterion.
3. Truncation: count the rows. If the file has exactly the query limit (5,000, 100,000, 200,000 or any round number that matches a limit) or if a single source-destination pair takes up most of the rows (a port scan, for example), declare it a truncated export, quantify how much the noise takes up and recommend repeating the export with filters and shorter windows. Do not claim that a host's traffic is complete if the export is truncated.
4. Deduplicate identical rows, separate unicast traffic from broadcast/multicast and discard anything that is not IP.

## Classification criterion for each peer without a VEN
- UNMANAGED WORKLOAD (UMWL): a specific server with a role identifiable from the evidence: DNS resolvers, internal NTP, monitoring servers and proxies (Zabbix, Nagios, SolarWinds, OEM), log collectors (Splunk, syslog), backup (NetBackup, Veeam, Commvault), databases (Oracle, SQL Server, MySQL, PostgreSQL), load balancers and front-ends of an application, cluster nodes, application servers, bastions or administrative SSH/RDP sources, vulnerability scanners, SMTP relays, message queues, domain controllers, agent managers (EDR, antivirus). One per IP.
- IP LIST: broad ranges or peers that are not servers: user and endpoint VLANs, whole application subnets, cloud metadata (169.254.169.254, which in OCI is also the VCN resolver), public NTP, SaaS consoles (EDR, monitoring), ranges published by a cloud provider, Internet.
- When a group of IPs shares role and application (front-ends, RAC nodes), propose a UMWL per IP with the same labels and, in addition, a convenience IP list to write the rule today and migrate to labels later.

## Evidence (mandatory for every inference)
Every role assigned to an IP without a VEN is an inference. For each UMWL and each list cite: ports and protocol, flow direction (who consumes whom), process and user on the VEN side, number of flows and bytes, time pattern (continuous, scheduled, one-off) and confidence level Alta / Media / Baja with one sentence of reasoning. If the role cannot be supported with evidence, mark it "(a confirmar)" and assign it priority 3.

## Label model
Four default types (Role, Application, Environment, Location) with a prefix in the value:
- R_<Role>   (R_DNS, R_NTP, R_Monitoring, R_LogCollector, R_Backup, R_Database, R_LoadBalancer, R_AppServer, R_Bastion, R_Scanner, R_Mail, R_Messaging, R_DomainController, R_AgentMgmt)
- A_<App>    (one per business application, plus A_CoreInfra, A_Observability, A_SecurityTools, A_AdminAccess)
- E_<Env>    (E_Prod, E_QA, E_Dev; if there is no evidence, E_Prod "a confirmar")
- L_<Loc>    (site or cloud: L_DC1, L_OCI, L_AWS, L_Azure, L_GCP)
If the PCE already has a convention (attached screenshots), respect it. Explain that the Application label is what supports the ringfence and that Illumio applies OR within a type and AND between types.

## Cybersecurity findings
List findings with ID (S-01…), severity (Alta/Media/Baja/Info), concrete evidence from the export and a recommendation. Look at least for: scans without a declared window, out-of-support software (deduced from processes/users/banners), cleartext protocols (FTP, Telnet, HTTP with credentials, SMB1), databases reached from public addressing, DNS storms or other anomalous volumes, direct public NTP/DNS from servers, direct SSH/RDP as root/Administrator, exposed management ports (AJP, JMX, RMI, WinRM), unexplained broadcast/multicast, agent inventory with its own egress. Every finding must be verifiable in Explorer with an IP and port filter.

## Fact verification
Before asserting a port, a process, a version, an end-of-support date or an Illumio behavior, verify it against current official documentation (product-docs-repo.illumio.com for Illumio; the vendor's documentation for Zabbix, Oracle, Splunk, NetBackup, Trend Micro, OCI/AWS/Azure) and cite the URL. Mark whatever you cannot verify as an assumption. Do not invent hostnames, people's names or commercial data.

## Deliverables
1. Report in Spanish, customer-oriented but WITHOUT the customer's name or people's names (use "el cliente", "la POC"), in HTML and PDF with the illumio-branded-reports skill. Sections: executive summary with indicators (rows, window, workloads with a VEN, unmanaged IPs, noise percentage), data and method (including normalization and truncation), inventory of workloads with a VEN and proposed labels, unmanaged workloads to create (table: name/FQDN, IP, inferred role and evidence, confidence, R/A/E/L labels, priority), internal networks and IP lists (table: name, members, reasoning, use in rules), label model, observed access patterns with a layered diagram, cybersecurity findings, proposed initial policy (services and ringfence rules per application plus a common management block), next steps, appendices and sources per section. Diagrams as inline SVG with the skill's palette, no rotated text and nothing outside the boxes.
2. XLSX workbook of objects to load with sheets: UMWL, IP lists, Labels, Services, Rules (draft) and Findings.
3. Unmanaged workload CSV in the workloader wkld-import format, ONE ROW PER IP, exact columns:
   hostname,name,interfaces,description,role,app,env,loc,review
   - hostname: always empty (the FQDNs in the export may be anonymized).
   - name: "<Descriptive role> <IP>", for example "Zabbix Server 10.43.43.21". Must be unique.
   - interfaces: "eth0:<ip>".
   - description: "[<group> P<priority> conf:<Alta|Media|Baja>] <comment with the evidence>", for example "[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP e ICMP en atun4 y wlsfp1a (≈600.000 flujos) y recibe 10051".
   - role, app, env, loc: values with the R_/A_/E_/L_ prefix. Empty cell if there is no label.
   - review: PENDING.
   Priority 1 = required for the ringfence (front-ends, databases, daily management plane); 2 = useful but confirmable later; 3 = identify before creating.
4. IP list CSV in the workloader ipl-import format, exact columns:
   name,description,include,exclude,fqdns
   - name in lowercase with the ipl- prefix (ipl-dns-corp, ipl-oci-metadata); include with IPs, CIDRs or ranges separated by ";"; fqdns for names.
5. A final "Supuestos y verificaciones pendientes" block with everything the customer must confirm before loading.

Deliver first a summary of what you found during normalization and a grouping proposal; then the report and the CSVs.
````

### Short variant: update with a new export (v2)

````text
I am attaching a new Explorer/Traffic export with the same scope as the previous analysis (I am also attaching the previous report and the umwl-import and ipl-import CSVs already loaded or proposed). I need the v2 version:

1. Normalize the new export with the same rules (mangled IPs, dates, truncation, deduplication) and compare window, rows and noise with the previous export.
2. Differences against the objects already proposed: new peers (propose a UMWL or a list with evidence, confidence and priority), peers that disappeared (mark them as "sin tráfico en la ventana nueva", do not delete), role or label changes suggested by the new evidence, new or closed findings.
3. Deliver:
   - <group>-umwl-import-v2.csv, complete (not only the delta), same columns and naming (empty hostname, name "<Role> <IP>", interfaces "eth0:<ip>", description "[<group> P<n> conf:X] …", R_/A_/E_/L_ labels, review = PENDING for new rows and UPDATED for rows whose comment or labels changed, UNCHANGED for the rest).
   - <group>-ipl-import-v2.csv, complete.
   - A table of changes (IP, before, after, reason) and an updated findings section.
   - If the full report is needed, generate it with the illumio-branded-reports skill; if not, a two-page addendum.
Verify against official documentation any new port, version or behavior you assert and cite the URL. No customer or people's names. Deliverables in Spanish.
````

After obtaining the v2 CSV, the TUI does the rest: in step 4 the rows whose IP already exists come out as EXISTS-UNMANAGED and are updated by `href` (labels and description); the new ones are created.

---

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| macOS does not let you run `workloader` ("cannot be opened because the developer cannot be verified") | Gatekeeper quarantine on the downloaded zip. `xattr -d com.apple.quarantine ./workloader && chmod +x ./workloader`. The TUI does this when it downloads. |
| `workloader` on Apple Silicon | The macOS release is an Intel binary and runs through Rosetta 2 (`softwareupdate --install-rosetta` if it is not installed). For a native binary: `brew install go`, `git clone https://github.com/brian1917/workloader.git && cd workloader && go build -o ../workloader .` (the TUI offers this option). |
| `pce-add` asks for email and password even though you passed `--api-user` | The `--api-key` flag is missing; without it workloader ignores `--api-user/--api-secret/--org`. The kit already includes it. |
| 401/403 when importing; the dry run works | The API key inherits the user's role. Create it with a Global Organization Owner or Global Administrator user (a scoped Workload Manager can create workloads, but not new labels or IP lists), or use a service account with those permissions. Check the `org id` too. |
| TLS error with an on-prem PCE and an internal certificate | `pce-add … --disable-tls-verification true` only in a lab; in production install the CA in the keychain. |
| The PCE shown by `pce-list` is not the customer's (or is another organization) | You are in the wrong folder (the current directory decides which `pce.yaml` is used) or `ILLUMIO_CONFIG` points to another file. One folder per account + PCE; `unset ILLUMIO_CONFIG`; `--pce <name>` only if the `pce.yaml` has several profiles. |
| The dry run says it will update a workload you did not expect | Match by `hostname` or `name` with an existing object. Check `runs/<ts>/dry-*.log`; rename the row (step 4/5) or let the IP reconciliation resolve it. |
| Duplicate names / "only one was loaded" | workloader treats two rows with the same `hostname` (or the same `name` without hostname) as the same workload. The TUI appends the IP as a suffix; by hand, make the `name` unique. |
| A label column was not applied | The label type does not exist in the PCE; `wkld-import` ignores the column. Create it in the console (Settings → Label Settings) or with `./workloader label-dimension-import types.csv --update-pce` (CSV with `key,display_name`), and run again. |
| The Explorer export has exactly N round rows | It is truncated. Raise the result limit, filter by application, exclude scanners, split the time window and export again (see the extraction section). |
| `wkld-import` creates the label with different capitalization | Values are case-sensitive unless `--ignore-case`. Use exactly the values shown by `label-export`. |
| Failed batch in step 7 | Check `runs/<ts>/create-chunkNNN.log`; the CSV of that batch stays in `create-chunkNNN.csv` to retry it by hand with `wkld-import … --umwl --update-pce`. |

---

## Explanatory guide (PDF)

The guide explains with diagrams the end-to-end flow (export → analysis → CSV → kit → workloader → PCE), the requirements and the one-folder-per-account + PCE rule, the initialization of the working folder, the sequence of TUI steps with their decision points, the relationship between the kit and the workloader subcommands, the mapping of CSV columns to PCE fields and labels, and troubleshooting. It is available in two languages, with the same content:

- English: [docs/Guide-workloader-import-kit-EN.pdf](docs/Guide-workloader-import-kit-EN.pdf) (self-contained HTML: `docs/Guide-workloader-import-kit-EN.html`).
- Español: [docs/Guia-workloader-import-kit.pdf](docs/Guia-workloader-import-kit.pdf) (HTML autocontenido: `docs/Guia-workloader-import-kit.html`).

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
