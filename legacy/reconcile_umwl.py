#!/usr/bin/env python3
"""
reconcile_umwl.py — match proposed unmanaged workloads against what already exists in the PCE, BY IP ADDRESS.

Workloader's wkld-import matches rows on href / hostname / name, never on interface IP. So before importing,
export the PCE inventory and reconcile:

    workloader wkld-export --output-file pce-workloads.csv
    python3 reconcile_umwl.py pce-workloads.csv cliente3-umwl-import.csv

Outputs (next to the proposed CSV):
    <proposed>-to-create.csv    rows whose IP is NOT in the PCE  -> workloader wkld-import <file> --umwl --update-pce
    <proposed>-existing.csv     rows whose IP already belongs to a workload; hostname/name/href rewritten to the
                                existing object so a normal wkld-import (no --umwl) only updates labels/description
    <proposed>-conflicts.csv    rows whose IP matches a MANAGED workload (VEN) or several workloads -> decide by hand
    <proposed>-reconcile-report.txt

Nothing is sent to the PCE by this script.
"""
import csv, re, sys, os, ipaddress
from collections import defaultdict

def parse_ips(s):
    out = []
    for tok in re.split(r'[;,\s]+', s or ''):
        tok = tok.strip()
        if not tok:
            continue
        tok = tok.split(':', 1)[1] if ':' in tok and not tok.count(':') > 1 else tok   # eth0:10.1.1.1 -> 10.1.1.1
        tok = tok.split('/')[0]                                                        # 10.1.1.1/24 -> 10.1.1.1
        try:
            ipaddress.ip_address(tok); out.append(tok)
        except ValueError:
            pass
    return out

def load_pce(path):
    """index: ip -> list of workload dicts (href, hostname, name, managed, labels)"""
    idx = defaultdict(list)
    with open(path, newline='', encoding='utf-8-sig') as f:
        rdr = csv.DictReader(f)
        for r in rdr:
            ips = parse_ips(r.get('interfaces', '')) + parse_ips(r.get('public_ip', ''))
            managed = (r.get('managed', '') or '').strip().lower() in ('true', 'yes', '1') or bool((r.get('ven_href') or '').strip())
            for ip in set(ips):
                idx[ip].append({'href': r.get('href', ''), 'hostname': r.get('hostname', ''), 'name': r.get('name', ''),
                                'managed': managed, 'role': r.get('role', ''), 'app': r.get('app', ''),
                                'env': r.get('env', ''), 'loc': r.get('loc', '')})
    return idx

def main(pce_csv, proposed_csv):
    idx = load_pce(pce_csv)
    base = re.sub(r'\.csv$', '', proposed_csv)
    with open(proposed_csv, newline='', encoding='utf-8-sig') as f:
        rdr = csv.DictReader(f); rows = list(rdr); fields = rdr.fieldnames
    create, existing, conflicts, report = [], [], [], []
    for r in rows:
        ips = parse_ips(r.get('interfaces', ''))
        matches = {m['href']: m for ip in ips for m in idx.get(ip, [])}
        if not matches:
            r['review'] = 'NEW'; create.append(r); continue
        if len(matches) > 1 or any(m['managed'] for m in matches.values()):
            m = next(iter(matches.values()))
            r['review'] = 'CONFLICT-MANAGED' if any(x['managed'] for x in matches.values()) else 'CONFLICT-MULTIPLE'
            r['existing_href'] = ';'.join(matches); r['existing_hostname'] = ';'.join(x['hostname'] for x in matches.values())
            r['existing_labels'] = ';'.join(f"{x['role']}/{x['app']}/{x['env']}/{x['loc']}" for x in matches.values())
            conflicts.append(r); continue
        m = next(iter(matches.values()))
        r['review'] = 'EXISTS-UNMANAGED'
        r['href'] = m['href']                       # exact match for wkld-import (no --umwl)
        r['hostname'] = m['hostname'] or r['hostname']
        r['proposed_name'] = r['name']; r['name'] = m['name'] or r['name']   # keep the PCE's visible name unless you edit it
        r['interfaces'] = ''                                                # blank = leave existing interfaces untouched
        r['existing_labels'] = f"{m['role']}/{m['app']}/{m['env']}/{m['loc']}"
        existing.append(r)
    extra = ['href', 'existing_href', 'existing_hostname', 'existing_labels']
    def dump(name, data, cols):
        p = f"{base}-{name}.csv"
        with open(p, 'w', newline='', encoding='utf-8') as f:
            w = csv.DictWriter(f, fieldnames=cols, extrasaction='ignore'); w.writeheader(); w.writerows(data)
        return p
    p1 = dump('to-create', create, fields)
    p2 = dump('existing', existing, ['href'] + [c for c in fields if c != 'interfaces'] + ['proposed_name', 'existing_labels'])
    p3 = dump('conflicts', conflicts, fields + extra)
    lines = [f"PCE inventory: {sum(len(v) for v in idx.values())} interface entries, {len(set(h['href'] for v in idx.values() for h in v))} workloads with IPs",
             f"Proposed rows: {len(rows)}", f"  NEW (create with --umwl):      {len(create)}  -> {p1}",
             f"  EXISTS-UNMANAGED (update):     {len(existing)}  -> {p2}",
             f"  CONFLICTS (decide by hand):    {len(conflicts)}  -> {p3}", "",
             "Next:", f"  workloader wkld-import {p1} --umwl            # dry run, read workloader.log",
             f"  workloader wkld-import {p1} --umwl --update-pce",
             f"  workloader wkld-import {p2}                   # labels/description only, matches on href"]
    open(f"{base}-reconcile-report.txt", 'w').write('\n'.join(lines)); print('\n'.join(lines))

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(__doc__); sys.exit(1)
    main(sys.argv[1], sys.argv[2])
