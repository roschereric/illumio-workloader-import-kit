#!/usr/bin/env python3
"""
umwl_loader.py — interactive loader of unmanaged workloads (and IP lists) into an Illumio PCE, on top of workloader.

    python3 umwl_loader.py sample-umwl-import.csv [--ipl sample-ipl-import.csv] [--pce NAME] [--priority 1]
                           [--workloader /path/to/workloader] [--chunk 20] [--runs ./runs]

What it does, step by step (every step is verbose and asks before anything touches the PCE):
  0. Preflight     find workloader, show which PCE will be used (pce-list), create runs/<timestamp>/
  1. Load & check  validate the proposed CSV (IPs, duplicates, required columns), filter by priority
  2. Inventory     wkld-export + label-export + label-dimension-export from the PCE (read-only)
  3. Labels        which label KEYS in the CSV do not exist as dimensions (drop / abort), which VALUES will be created
  4. Reconcile     match proposed rows against the PCE BY IP ADDRESS -> NEW / EXISTS-UNMANAGED / CONFLICT, and ask
                   what to do with every existing/conflicting row (update, skip, create anyway, rename)
  5. Review NEW    accept all or walk row by row editing name / description / labels
  6. Dry run       workloader wkld-import without --update-pce; shows what workloader itself intends to do
  7. Execute       chunked imports with a progress bar; on error: retry / skip chunk / abort
  8. Verify        re-export and confirm every created IP is now a workload
  9. IP lists      (optional) same dry-run / confirm / import for the ipl CSV
 10. Report        runs/<timestamp>/report.md + report.json + all CSVs and workloader logs

Only the Python standard library is used. Tested against workloader's wkld-import / ipl-import CSV contracts
(headers: hostname, name, interfaces, description, <label keys>; ipl: name, description, include, exclude, fqdns).
"""
import argparse, csv, datetime as dt, ipaddress, json, os, re, shutil, subprocess, sys, textwrap
from collections import defaultdict, Counter

# ----------------------------------------------------------------------------- terminal helpers
USE_COLOR = sys.stdout.isatty()
def c(code, s): return f"\033[{code}m{s}\033[0m" if USE_COLOR else str(s)
B = lambda s: c("1", s); DIM = lambda s: c("2", s)
RED = lambda s: c("31", s); GRN = lambda s: c("32", s); YEL = lambda s: c("33", s); CYA = lambda s: c("36", s); ORA = lambda s: c("38;5;208", s)
W = min(shutil.get_terminal_size((100, 30)).columns, 110)

def banner(n, title):
    print("\n" + ORA("━" * W)); print(ORA(B(f" PASO {n} · {title}"))); print(ORA("━" * W))
def info(s): print(f"  {CYA('•')} {s}")
def ok(s): print(f"  {GRN('✔')} {s}")
def warn(s): print(f"  {YEL('▲')} {s}")
def err(s): print(f"  {RED('✖')} {s}")
def wrap(s, indent=4): return textwrap.fill(s, W - indent, initial_indent=" " * indent, subsequent_indent=" " * indent)

def ask(prompt, choices, default=None):
    """choices: dict letter -> description. Returns the letter."""
    legend = "  ".join(f"[{B(k)}] {v}" for k, v in choices.items())
    while True:
        try:
            a = input(f"  {prompt}\n    {legend}\n    > ").strip().lower()
        except (EOFError, KeyboardInterrupt):
            print(); return "q"
        if not a and default: return default
        if a in choices: return a
        print(f"    {DIM('opción inválida')}")

def ask_text(prompt, current=""):
    try:
        a = input(f"    {prompt} [{current}]: ").strip()
    except (EOFError, KeyboardInterrupt):
        return current
    return a or current

def progress(done, total, label=""):
    bar_w = 40; filled = int(bar_w * done / max(total, 1))
    sys.stdout.write(f"\r  {ORA('█' * filled)}{DIM('░' * (bar_w - filled))} {done}/{total} {label:<40}")
    sys.stdout.flush()
    if done >= total: print()

def table(rows, headers, maxw=None):
    if not rows: return
    cols = list(zip(*([headers] + rows)))
    widths = [min(max(len(str(x)) for x in col), maxw or 60) for col in cols]
    fmt = "  " + "  ".join("{:<%d}" % w for w in widths)
    print(DIM(fmt.format(*[h[:w] for h, w in zip(headers, widths)])))
    for r in rows: print(fmt.format(*[str(x)[:w] for x, w in zip(r, widths)]))

# ----------------------------------------------------------------------------- workloader wrapper
class Workloader:
    def __init__(self, binary, pce, run_dir):
        self.bin, self.pce, self.run_dir = binary, pce, run_dir
        self.calls = []
    def run(self, args, log_name, check=True):
        log = os.path.join(self.run_dir, log_name)
        cmd = [self.bin] + args + ["--log-file", log]
        if self.pce: cmd += ["--pce", self.pce]
        info(DIM("$ " + " ".join(cmd)))
        p = subprocess.run(cmd, capture_output=True, text=True)
        self.calls.append({"cmd": cmd, "rc": p.returncode, "log": log})
        if p.stdout.strip():
            for line in p.stdout.strip().splitlines()[-12:]: print("    " + DIM(line.rstrip()))
        if p.returncode != 0 and check:
            err(f"workloader terminó con código {p.returncode}")
            if p.stderr.strip(): print(wrap(p.stderr.strip()[-800:]))
        return p, log

def read_csv(path):
    with open(path, newline="", encoding="utf-8-sig") as f:
        r = csv.DictReader(f); return list(r), r.fieldnames
def write_csv(path, rows, fields):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=fields, extrasaction="ignore"); w.writeheader(); w.writerows(rows)

def parse_ips(s):
    out = []
    for tok in re.split(r"[;,\s]+", s or ""):
        tok = tok.strip()
        if not tok: continue
        if ":" in tok and tok.count(":") == 1: tok = tok.split(":", 1)[1]
        tok = tok.split("/")[0]
        try: ipaddress.ip_address(tok); out.append(tok)
        except ValueError: pass
    return out

def log_lines(path, pattern):
    if not os.path.exists(path): return []
    with open(path, encoding="utf-8", errors="replace") as f:
        return [l.rstrip() for l in f if re.search(pattern, l)]


# ----------------------------------------------------------------------------- bootstrap helpers
REPO = "https://github.com/brian1917/workloader"

def sh(cmd, **kw):
    info(DIM("$ " + " ".join(cmd)))
    return subprocess.run(cmd, **kw)

def latest_tag():
    """Resolve the latest release tag from the GitHub redirect (no API token needed)."""
    try:
        p = subprocess.run(["curl", "-sI", "-o", "/dev/null", "-w", "%{redirect_url}", f"{REPO}/releases/latest"], capture_output=True, text=True, timeout=20)
        m = re.search(r"/tag/(v[\d.]+)", p.stdout); return m.group(1) if m else None
    except Exception:
        return None

def bootstrap_workloader(explicit):
    """Find ./workloader (or PATH / explicit). If absent, offer to download the release or clone+build."""
    candidates = [explicit] if explicit else ["./workloader", shutil.which("workloader")]
    for cnd in candidates:
        if cnd and os.path.isfile(cnd) and os.access(cnd, os.X_OK):
            p = subprocess.run([cnd, "version"], capture_output=True, text=True)
            ok(f"workloader encontrado: {cnd} — {(p.stdout or p.stderr).strip().splitlines()[0] if (p.stdout or p.stderr).strip() else ''}")
            return os.path.abspath(cnd)
    warn("no hay un binario 'workloader' en esta carpeta ni en el PATH")
    is_mac = sys.platform == "darwin"; arch = os.uname().machine
    tag = latest_tag()
    info(f"último release publicado: {tag or 'no pude resolverlo (sin red?)'} · sistema: {sys.platform}/{arch}")
    if is_mac and arch == "arm64":
        info("el release de macOS se compila para Intel (GOOS=darwin sin GOARCH); en Apple Silicon corre vía Rosetta 2: preferí compilarlo nativo con Go (opción c)")
    while True:
        ch = ask("¿Cómo lo instalamos?", {"c": f"git clone {tag or ''} en ./workloader-src + go build (binario nativo, recomendado; requiere Go y git)",
                                          "d": f"descargar el release {tag or 'latest'} en esta carpeta (curl + unzip; Intel/Rosetta en Apple Silicon)",
                                          "m": "mostrar los comandos y salir", "q": "salir"}, "c" if shutil.which("go") else "d")
        if ch == "q": sys.exit(0)
        if ch == "m":
            print(wrap(f"Descarga: curl -LO {REPO}/releases/download/{tag or '<tag>'}/mac-{tag or '<tag>'}.zip && unzip mac-{tag or '<tag>'}.zip && mv mac-{tag or '<tag>'}/workloader . && chmod +x workloader && xattr -d com.apple.quarantine workloader"))
            print(wrap(f"Build:    git clone --depth 1 --branch {tag or '<tag>'} {REPO}.git workloader-src && cd workloader-src && CGO_ENABLED=0 go build -trimpath -ldflags \"-s -w -X github.com/brian1917/workloader/utils.Version=$(cat version) -X github.com/brian1917/workloader/utils.Commit=$(git rev-list -1 HEAD)\" -o ../workloader . && cd .."))
            sys.exit(0)
        if ch == "d":
            if not tag: err("no pude resolver el tag; probá la opción c o m"); continue
            asset = {"darwin": f"mac-{tag}.zip", "linux": f"linux_amd64-{tag}.zip"}.get(sys.platform, f"mac-{tag}.zip")
            url = f"{REPO}/releases/download/{tag}/{asset}"
            if sh(["curl", "-fL", "-o", asset, url]).returncode != 0: err("descarga fallida"); continue
            if sh(["unzip", "-o", "-q", asset]).returncode != 0: err("unzip falló"); continue
            src = next((os.path.join(d, "workloader") for d in [asset[:-4], "."] if os.path.isfile(os.path.join(d, "workloader"))), None)
            if not src: err("no encontré el binario dentro del zip"); continue
            if os.path.abspath(src) != os.path.abspath("./workloader"): shutil.move(src, "./workloader")
            os.chmod("./workloader", 0o755)
            if is_mac: subprocess.run(["xattr", "-d", "com.apple.quarantine", "./workloader"], capture_output=True)
            ok("binario instalado en ./workloader (quarantine de Gatekeeper eliminada)")
            return os.path.abspath("./workloader")
        if ch == "c":
            if not shutil.which("go"): err("Go no está instalado (brew install go) — usá la opción d"); continue
            if not shutil.which("git"): err("git no está instalado"); continue
            if not os.path.isdir("workloader-src"):
                clone = ["git", "clone", "--depth", "1"] + (["--branch", tag] if tag else []) + [f"{REPO}.git", "workloader-src"]
                if sh(clone).returncode != 0: err("clone falló"); continue
            ver = open("workloader-src/version").read().strip() if os.path.exists("workloader-src/version") else ""
            commit = subprocess.run(["git", "-C", "workloader-src", "rev-list", "-1", "HEAD"], capture_output=True, text=True).stdout.strip()
            ldflags = f"-s -w -X github.com/brian1917/workloader/utils.Version={ver} -X github.com/brian1917/workloader/utils.Commit={commit}"
            env = dict(os.environ, CGO_ENABLED="0")
            if subprocess.run(["go", "build", "-trimpath", "-ldflags", ldflags, "-o", "../workloader", "."], cwd="workloader-src", env=env).returncode != 0: err("go build falló"); continue
            os.chmod("./workloader", 0o755); ok(f"binario {ver} compilado nativo en ./workloader")
            return os.path.abspath("./workloader")

def ensure_pce(wl):
    """Make sure pce.yaml has a PCE; otherwise guide through pce-add (API key)."""
    cfg = os.environ.get("ILLUMIO_CONFIG") or "./pce.yaml"
    p, _ = wl.run(["pce-list"], "pce-list.log", check=False)
    none_msg = "no pce configured" in (p.stdout + p.stderr).lower()   # workloader prints this with rc=0 when pce.yaml has no entries
    have = p.returncode == 0 and os.path.exists(cfg) and os.path.getsize(cfg) > 0 and not none_msg
    if none_msg: warn("workloader no tiene ningún PCE configurado en esta carpeta: vamos directo a pce-add")
    if have and ask("¿Es este el PCE correcto?", {"s": "sí, continuar", "a": "agregar/cambiar PCE (pce-add)", "q": "salir"}, "s") == "s": return
    if not have and not none_msg: warn(f"no hay PCE configurado ({cfg} ausente o vacío). workloader lee ./pce.yaml, $ILLUMIO_CONFIG o --config-file")
    print(wrap("Necesitás un API key del PCE: en la consola, menú de usuario → My API Keys → Add. Guardá 'Authentication Username' (api_…) "
               "y 'Secret'. Para SaaS, el org id es el número que aparece en la URL (…/orgs/<id>/…) y el FQDN es el host de la consola; puerto 443. "
               "El kit siempre usa pce-add --api-key: nunca pide email/contraseña."))
    ch = ask("¿Cómo lo configuramos?", {"k": "ingresar los datos acá (API key)", "i": "correr 'workloader pce-add --api-key' interactivo", "q": "salir"}, "k")
    if ch == "q": sys.exit(0)
    if ch == "i":
        # always --api-key: without it workloader asks email/password and tries to log in via login.illum.io
        # (fails with 401 for SSO users). With it, it prompts: API Authentication Username, API Secret, Org.
        p = subprocess.run([wl.bin, "pce-add", "--api-key"])
        if p.returncode != 0: err("pce-add falló"); sys.exit(1)
    else:
        import getpass
        name = ask_text("nombre corto del PCE", "poc"); fqdn = ask_text("FQDN del PCE", "xxx.illum.io"); port = ask_text("puerto", "443")
        user = ask_text("API user (api_…)", ""); secret = getpass.getpass("    API secret: "); org = ask_text("org id", "1")
        tls = ask_text("desactivar verificación TLS? (true/false)", "false")
        p = subprocess.run([wl.bin, "pce-add", "--api-key", "--name", name, "--fqdn", fqdn, "--port", port, "--api-user", user, "--api-secret", secret, "--org", org, "--disable-tls-verification", tls], text=True)
        if p.returncode != 0: err("pce-add falló"); sys.exit(1)
    p, _ = wl.run(["pce-list"], "pce-list.log", check=False)
    if p.returncode != 0: err("sigue sin poder listar el PCE"); sys.exit(1)
    p, _ = wl.run(["label-dimension-export", "--output-file", os.path.join(wl.run_dir, "conn-test.csv")], "conn-test.log", check=False)
    (ok if p.returncode == 0 else err)("prueba de conexión al PCE (label-dimension-export): " + ("OK" if p.returncode == 0 else "falló — revisá FQDN/API key/org"))
    if p.returncode != 0: sys.exit(1)

# ----------------------------------------------------------------------------- main flow
def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("csv", nargs="?", help="proposed unmanaged workloads CSV (one row per IP)")
    ap.add_argument("--setup-only", action="store_true", help="only install/verify workloader and the PCE connection")
    ap.add_argument("--ipl", help="proposed IP lists CSV (workloader ipl-import format)")
    ap.add_argument("--pce", help="PCE name in pce.yaml (workloader --pce)")
    ap.add_argument("--priority", help="only load rows whose description starts with [.. P<n> ..] for these priorities, e.g. 1 or 1,2")
    ap.add_argument("--workloader", default=None, help="path to the workloader binary (default: ./workloader, then PATH)")
    ap.add_argument("--chunk", type=int, default=20, help="rows per wkld-import call (progress granularity / blast radius)")
    ap.add_argument("--runs", default="./runs")
    a = ap.parse_args()

    ts = dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    run_dir = os.path.join(a.runs, ts); os.makedirs(run_dir, exist_ok=True)
    report = {"started": ts, "csv": a.csv, "pce": a.pce, "steps": [], "created": [], "updated": [], "skipped": [], "conflicts": [],
              "labels_created": [], "failed_chunks": [], "iplists": {}, "verify": {}}

    # ---- 0 preflight: binary, Gatekeeper, pce.yaml
    banner(0, "Preflight: workloader y conexión al PCE")
    binary = bootstrap_workloader(a.workloader)
    wl = Workloader(binary, a.pce, run_dir)
    ok(f"workloader: {binary}"); ok(f"carpeta de la corrida: {run_dir}")
    ensure_pce(wl)
    if a.setup_only:
        ok("setup completo. Volvé a correr con el CSV cuando lo tengas."); return
    if not a.csv: err("falta el CSV propuesto (o usá --setup-only)"); sys.exit(1)

    # ---- 1 load
    banner(1, "Cargar y validar el CSV propuesto")
    rows, fields = read_csv(a.csv)
    required = {"hostname", "name", "interfaces"}
    missing = required - set(fields)
    if missing: err(f"faltan columnas {missing}"); sys.exit(1)
    label_cols = [f for f in fields if f not in ("hostname", "name", "interfaces", "description", "review", "href", "public_ip", "os_id", "os_detail", "data_center", "external_data_set", "external_data_reference")]
    if a.priority:
        keep = set(a.priority.split(","))
        before = len(rows); rows = [r for r in rows if re.search(r"\bP(\d)\b", r.get("description", "")) and re.search(r"\bP(\d)\b", r["description"]).group(1) in keep]
        info(f"filtro de prioridad {sorted(keep)}: {before} → {len(rows)} filas")
    bad = [r for r in rows if not parse_ips(r["interfaces"])]
    dup = [ip for ip, n in Counter(ip for r in rows for ip in parse_ips(r["interfaces"])).items() if n > 1]
    empty_cols = [k for k in label_cols if not any(r.get(k, "").strip() for r in rows)]
    if empty_cols: info(f"columnas de etiqueta vacías, se ignoran: {empty_cols}"); label_cols = [k for k in label_cols if k not in empty_cols]
    ok(f"{len(rows)} filas; columnas de etiqueta detectadas: {label_cols}")
    if bad: warn(f"{len(bad)} filas sin IP válida (se omiten): " + ", ".join(r['name'] for r in bad)); rows = [r for r in rows if r not in bad]
    if dup: warn(f"IPs repetidas dentro del CSV: {dup} — se carga la primera aparición"); seen = set(); rows = [r for r in rows if not (parse_ips(r['interfaces'])[0] in seen or seen.add(parse_ips(r['interfaces'])[0]))]
    blank_h = sum(1 for r in rows if not r["hostname"].strip())
    if blank_h:
        info(f"{blank_h} filas sin hostname: workloader las identifica por 'name' (convención 'Rol IP'); el hostname queda vacío en el PCE")
        empty_name = [r for r in rows if not r["hostname"].strip() and not r["name"].strip()]
        if empty_name: warn(f"{len(empty_name)} filas sin hostname NI name (se omiten)"); rows = [r for r in rows if r not in empty_name]
    seen_h = Counter(r["hostname"] for r in rows if r["hostname"].strip())
    duph = [h for h, n in seen_h.items() if n > 1]
    if duph:
        warn(f"hostnames repetidos (workloader los tomaría como el MISMO workload): {duph} — se les agrega la IP como sufijo")
        for r in rows:
            if r["hostname"] in duph: r["hostname"] = f"{r['hostname']}--{parse_ips(r['interfaces'])[0]}"
    seen_n = Counter(r["name"] for r in rows if not r["hostname"].strip())
    dupn = [h for h, n in seen_n.items() if n > 1]
    if dupn:
        warn(f"names repetidos en filas sin hostname (workloader los tomaría como el MISMO workload): {dupn} — se les agrega la IP como sufijo")
        for r in rows:
            if not r["hostname"].strip() and r["name"] in dupn: r["name"] = f"{r['name']} {parse_ips(r['interfaces'])[0]}"
    for k in label_cols:
        vals = Counter(r.get(k, "") for r in rows if r.get(k, ""))
        info(f"{k}: " + ", ".join(f"{v} ({n})" for v, n in vals.most_common(8)) + (" …" if len(vals) > 8 else ""))
    report["steps"].append({"step": 1, "rows": len(rows)})

    # ---- 2 inventory
    banner(2, "Inventario del PCE (solo lectura)")
    inv_csv = os.path.join(run_dir, "pce-workloads.csv")
    p, _ = wl.run(["wkld-export", "--output-file", inv_csv], "wkld-export.log")
    if p.returncode != 0 and ask("Sin inventario no se puede reconciliar por IP. ¿Reintentar?", {"r": "reintentar", "q": "salir"}, "r") == "r":
        p, _ = wl.run(["wkld-export", "--output-file", inv_csv], "wkld-export.log")
    if p.returncode != 0: sys.exit(1)
    inv, _ = read_csv(inv_csv)
    idx = defaultdict(list)
    for r in inv:
        managed = (r.get("managed", "") or "").strip().lower() in ("true", "yes", "1") or bool((r.get("ven_href") or "").strip())
        for ip in set(parse_ips(r.get("interfaces", "")) + parse_ips(r.get("public_ip", ""))):
            idx[ip].append({"href": r.get("href", ""), "hostname": r.get("hostname", ""), "name": r.get("name", ""), "managed": managed,
                            "labels": {k: r.get(k, "") for k in label_cols}})
    ok(f"{len(inv)} workloads en el PCE ({sum(1 for r in inv if (r.get('managed','').lower()=='true' or r.get('ven_href')))} gestionados), {len(idx)} IPs indexadas")
    lab_csv = os.path.join(run_dir, "pce-labels.csv"); dim_csv = os.path.join(run_dir, "pce-label-dimensions.csv")
    wl.run(["label-export", "--output-file", lab_csv], "label-export.log", check=False)
    wl.run(["label-dimension-export", "--output-file", dim_csv], "label-dimension-export.log", check=False)
    labels = read_csv(lab_csv)[0] if os.path.exists(lab_csv) else []
    dims = [d.get("key", "") for d in read_csv(dim_csv)[0]] if os.path.exists(dim_csv) else ["role", "app", "env", "loc"]
    existing_labels = {(l.get("key", ""), l.get("value", "")) for l in labels}
    ok(f"{len(labels)} etiquetas y tipos {dims} en el PCE")

    # ---- 3 labels
    banner(3, "Etiquetas: tipos y valores")
    unknown_keys = [k for k in label_cols if k not in dims]
    if unknown_keys:
        warn(f"columnas que NO son un tipo de etiqueta del PCE: {unknown_keys} (workloader las ignoraría en silencio)")
        ch = ask("¿Qué hacemos con esas columnas?", {"d": "descartarlas", "q": "salir y crear el tipo primero (label-dimension-import)"}, "d")
        if ch == "q": sys.exit(0)
        label_cols = [k for k in label_cols if k in dims]
    new_vals = sorted({(k, r[k]) for r in rows for k in label_cols if r.get(k) and (k, r[k]) not in existing_labels})
    if new_vals:
        info(f"{len(new_vals)} etiquetas nuevas se crearán durante el import:")
        table([[k, v] for k, v in new_vals], ["tipo", "valor"])
        if ask("¿Confirmás la creación de estas etiquetas?", {"s": "sí", "q": "salir a corregir el CSV"}, "s") == "q": sys.exit(0)
    else: ok("todas las etiquetas del CSV ya existen")
    report["labels_planned"] = [f"{k}={v}" for k, v in new_vals]

    # ---- 4 reconcile by IP
    banner(4, "Reconciliación por IP contra el PCE")
    create, update, skipped, conflicts = [], [], [], []
    bulk_choice = {}
    for r in rows:
        ips = parse_ips(r["interfaces"]); matches = {m["href"]: m for ip in ips for m in idx.get(ip, [])}
        if not matches: r["review"] = "NEW"; create.append(r); continue
        kind = "CONFLICT-MANAGED" if any(m["managed"] for m in matches.values()) else ("CONFLICT-MULTIPLE" if len(matches) > 1 else "EXISTS-UNMANAGED")
        m = next(iter(matches.values()))
        print(); print(f"  {YEL(kind)}  IP {B(ips[0])}")
        table([["propuesto", r["hostname"], r["name"]] + [r.get(k, "") for k in label_cols],
               ["en el PCE", m["hostname"], m["name"]] + [m["labels"].get(k, "") for k in label_cols]],
              ["", "hostname", "name"] + label_cols, maxw=28)
        if len(matches) > 1: warn("varias workloads comparten esta IP: " + "; ".join(f"{x['hostname']} ({x['href']})" for x in matches.values()))
        ch = bulk_choice.get(kind)
        if not ch:
            opts = {"u": "actualizar etiquetas/descripción del existente", "s": "omitir", "c": "crear igual (duplicado)", "r": "renombrar y actualizar",
                    "a": "aplicar 'actualizar' a todos los " + kind, "o": "aplicar 'omitir' a todos los " + kind, "q": "abortar"}
            if kind == "CONFLICT-MANAGED": opts["u"] = "actualizar etiquetas del workload GESTIONADO (cuidado)"
            ch = ask("¿Qué hacemos?", opts, "s" if kind.startswith("CONFLICT") else "u")
            if ch == "a": bulk_choice[kind] = "u"; ch = "u"
            if ch == "o": bulk_choice[kind] = "s"; ch = "s"
        if ch == "q": sys.exit(0)
        if ch == "s": r["review"] = "SKIPPED-" + kind; skipped.append(r); continue
        if ch == "c": r["review"] = "NEW-DUP"; create.append(r); continue
        if ch == "r": r["name"] = ask_text("nuevo nombre visible", r["name"])
        elif ch == "u": r["name"] = m["name"] or r["name"]
        r["review"] = "UPDATE-" + kind; r["href"] = m["href"]; r["hostname"] = m["hostname"] or r["hostname"]; r["interfaces"] = ""
        update.append(r)
    ok(f"nuevos: {len(create)} · a actualizar: {len(update)} · omitidos: {len(skipped)}")

    # ---- 5 review new rows
    banner(5, "Revisión de los workloads nuevos")
    if create:
        table([[r["interfaces"].replace("umwl:", ""), r["hostname"], r["name"]] + [r.get(k, "") for k in label_cols] for r in create[:60]], ["ip", "hostname", "name"] + label_cols, maxw=26)
        if len(create) > 60: info(f"… {len(create) - 60} más")
        if ask("¿Aceptar todos o revisar uno por uno?", {"a": "aceptar todos", "r": "revisar uno por uno", "q": "abortar"}, "a") == "r":
            kept = []
            for i, r in enumerate(create, 1):
                print(f"\n  [{i}/{len(create)}] {B(r['interfaces'])}  {r['name']}"); print(wrap(r.get("description", "")))
                ch = ask("", {"k": "conservar", "e": "editar", "s": "omitir", "a": "aceptar el resto", "q": "abortar"}, "k")
                if ch == "q": sys.exit(0)
                if ch == "a": kept += create[i - 1:]; break
                if ch == "s": r["review"] = "SKIPPED-USER"; skipped.append(r); continue
                if ch == "e":
                    r["name"] = ask_text("name", r["name"]); r["hostname"] = ask_text("hostname", r["hostname"]); r["description"] = ask_text("description", r.get("description", ""))
                    for k in label_cols: r[k] = ask_text(k, r.get(k, ""))
                kept.append(r)
            create = kept
    out_fields = ["hostname", "name", "interfaces", "description"] + label_cols
    create_csv = os.path.join(run_dir, "to-create.csv"); update_csv = os.path.join(run_dir, "to-update.csv"); skip_csv = os.path.join(run_dir, "skipped.csv")
    write_csv(create_csv, create, out_fields); write_csv(update_csv, update, ["href"] + out_fields); write_csv(skip_csv, skipped, out_fields + ["review"])
    ok(f"CSVs finales escritos en {run_dir}")

    # ---- 6 dry run
    banner(6, "Dry run (workloader sin --update-pce)")
    # workloader elige la COLUMNA de matching por prioridad (href, hostname, name) y descarta las filas con esa
    # columna vacía; no cae a name. Con hostnames vacíos hay que pedir --match name; para actualizar, --match href.
    create_extra = ["--umwl", "--update=false"] + (["--match", "name"] if any(not r["hostname"].strip() for r in create) else [])
    update_extra = ["--match", "href"]
    def dry(path, extra, name):
        if not read_csv(path)[0]: info(f"{name}: nada que hacer"); return True
        p, log = wl.run(["wkld-import", path] + extra, f"dry-{name}.log")
        bad = log_lines(log, r"cannot be blank|nothing to be done|\[WARN|\[ERROR")
        for l in log_lines(log, r"to be created|needs to be created|to be changed|is not a workload|cannot be blank|nothing to be done|\[WARN|\[ERROR"): print("    " + DIM(l[:W - 6]))
        if bad: warn(f"{name}: workloader descartó filas o no haría nada ({len(bad)} líneas); revisá el log antes de ejecutar")
        return p.returncode == 0 and not bad
    d1 = dry(create_csv, create_extra, "create"); d2 = dry(update_csv, update_extra, "update")
    if not (d1 and d2):
        if ask("El dry run reportó errores.", {"c": "continuar de todos modos", "q": "abortar"}, "q") == "q": sys.exit(1)
    if ask(f"¿Ejecutar contra el PCE? Se crearán {len(create)} y actualizarán {len(update)} workloads.", {"s": "sí, ejecutar", "q": "no, terminar aquí"}, "q") == "q":
        finish(report, run_dir, create, update, skipped, wl, aborted=True); return

    # ---- 7 execute in chunks
    banner(7, "Ejecución por lotes")
    def execute(rows_, fields_, extra, name, bucket):
        chunks = [rows_[i:i + a.chunk] for i in range(0, len(rows_), a.chunk)]
        for n, ch in enumerate(chunks, 1):
            cp = os.path.join(run_dir, f"{name}-chunk{n:03d}.csv"); write_csv(cp, ch, fields_)
            while True:
                progress(n - 1, len(chunks), f"lote {n} ({len(ch)} filas)"); print()
                p, log = wl.run(["wkld-import", cp, "--update-pce", "--no-prompt"] + extra, f"{name}-chunk{n:03d}.log", check=False)
                created = log_lines(log, r"created new .* label"); okl = log_lines(log, r"bulk (create|update) workload successful"); errl = log_lines(log, r"\[ERROR|\[WARN|cannot be blank|nothing to be done")
                for l in created: report["labels_created"].append(l.split(" - ", 1)[-1])
                if p.returncode == 0 and not errl:
                    bucket.extend(ch); break
                err(f"lote {n} con problemas (rc={p.returncode})"); [print("    " + DIM(l[:W - 6])) for l in errl[-6:]]
                c2 = ask("", {"r": "reintentar lote", "s": "saltar lote", "q": "abortar"}, "r")
                if c2 == "s": report["failed_chunks"].append({"file": cp, "log": log}); break
                if c2 == "q": report["failed_chunks"].append({"file": cp, "log": log}); progress(len(chunks), len(chunks)); return False
        progress(len(chunks), len(chunks), "listo"); return True
    if create: execute(create, out_fields, create_extra, "create", report["created"])
    if update: execute(update, ["href"] + out_fields, update_extra, "update", report["updated"])

    # ---- 8 verify
    banner(8, "Verificación")
    ver_csv = os.path.join(run_dir, "pce-workloads-after.csv")
    p, _ = wl.run(["wkld-export", "--output-file", ver_csv], "wkld-export-after.log", check=False)
    if p.returncode == 0:
        after = set(ip for r in read_csv(ver_csv)[0] for ip in parse_ips(r.get("interfaces", "")))
        missing = [r for r in report["created"] if parse_ips(r["interfaces"])[0] not in after]
        report["verify"] = {"created_expected": len(report["created"]), "missing": [r["interfaces"] for r in missing]}
        (ok if not missing else err)(f"{len(report['created']) - len(missing)}/{len(report['created'])} IPs creadas confirmadas en el PCE" + (f"; faltan: {[r['interfaces'] for r in missing]}" if missing else ""))

    # ---- 9 IP lists
    if a.ipl:
        banner(9, "Listas de IP")
        p, log = wl.run(["ipl-import", a.ipl], "dry-ipl.log", check=False)
        for l in log_lines(log, r"create|update|\[WARN|\[ERROR"): print("    " + DIM(l[:W - 6]))
        if ask("¿Importar las listas de IP?", {"s": "sí", "n": "no"}, "s") == "s":
            p, log = wl.run(["ipl-import", a.ipl, "--update-pce", "--no-prompt"], "ipl-import.log", check=False)
            report["iplists"] = {"rc": p.returncode, "log": log, "rows": len(read_csv(a.ipl)[0])}
            (ok if p.returncode == 0 else err)(f"ipl-import rc={p.returncode}")

    finish(report, run_dir, create, update, skipped, wl)

def finish(report, run_dir, create, update, skipped, wl, aborted=False):
    banner(10, "Reporte")
    report["finished"] = dt.datetime.now().strftime("%Y%m%d-%H%M%S"); report["aborted"] = aborted; report["commands"] = wl.calls
    report["skipped"] = [{"ip": r["interfaces"], "name": r["name"], "why": r.get("review")} for r in skipped]
    md = [f"# Carga de unmanaged workloads — {report['started']}", "", f"- CSV origen: `{report['csv']}`", f"- PCE: `{report.get('pce') or 'default (pce.yaml)'}`",
          f"- Estado: {'ABORTADO antes de escribir en el PCE' if aborted else 'ejecutado'}", "",
          "## Resumen", "", f"| Creados | Actualizados | Omitidos | Lotes fallidos | Etiquetas creadas |", "|---|---|---|---|---|",
          f"| {len(report['created'])} | {len(report['updated'])} | {len(report['skipped'])} | {len(report['failed_chunks'])} | {len(report['labels_created'])} |", ""]
    if report.get("verify"): md += [f"Verificación post-carga: {report['verify']}", ""]
    if report["labels_created"]: md += ["## Etiquetas creadas", ""] + [f"- {l}" for l in report["labels_created"]] + [""]
    for title, rows in (("Creados", report["created"]), ("Actualizados", report["updated"])):
        if rows: md += [f"## {title}", "", "| IP | hostname | name |", "|---|---|---|"] + [f"| {r.get('interfaces','')} | {r['hostname']} | {r['name']} |" for r in rows] + [""]
    if report["skipped"]: md += ["## Omitidos", ""] + [f"- {s['ip']} {s['name']} — {s['why']}" for s in report["skipped"]] + [""]
    if report["failed_chunks"]: md += ["## Lotes con error", ""] + [f"- {f['file']} → {f['log']}" for f in report["failed_chunks"]] + [""]
    if report.get("iplists"): md += ["## Listas de IP", "", f"- {report['iplists']}", ""]
    md += ["## Comandos ejecutados", ""] + [f"- rc={c['rc']} `{' '.join(c['cmd'])}`" for c in wl.calls]
    open(os.path.join(run_dir, "report.md"), "w", encoding="utf-8").write("\n".join(md))
    json.dump(report, open(os.path.join(run_dir, "report.json"), "w", encoding="utf-8"), indent=2, ensure_ascii=False, default=str)
    ok(f"reporte: {os.path.join(run_dir, 'report.md')}"); print("\n".join(md[6:12]))

if __name__ == "__main__":
    main()
