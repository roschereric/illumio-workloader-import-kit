#!/usr/bin/env python3
"""
anonymize_export.py — consistent, reversible pseudonymization of an Illumio Explorer/Traffic export
(and of any CSV produced from it) before sharing it with an AI assistant or a third party.

    python3 anonymize_export.py anon  TrafficData.csv  -o TrafficData.anon.csv  --map anon-map.json \
        [--customer "Acme S.A." --customer "acme"] [--domain acme.com --domain corp.acme.local] [--public-ips]
    python3 anonymize_export.py deanon  proposed-umwl-import.csv -o proposed-umwl-import.real.csv --map anon-map.json

What is replaced (same input → same token, across every column and every run that shares the map file):
  - hostnames / FQDN / names in the Source|Destination Name, Hostname, FQDN columns → host-0001.company.com style
    (the domain part is mapped to company.com / dept.company.com so tiers stay recognizable)
  - customer names given with --customer (case-insensitive, whole words) → "Cliente"
  - domains given with --domain (and their sub-domains) → company.com, dept.company.com
  - usernames (Consuming Username, Username) → user-01 …, except well-known service accounts (root, oracle, zabbix,
    nobody, www-data, …) which are kept because they carry the role evidence the analysis needs
  - public IPs → 203.0.113.x / 198.51.100.x test ranges, only with --public-ips (RFC 1918 / CGNAT / link-local stay
    intact: they are the join key for the unmanaged workloads and IP lists)
What is NOT replaced: private IPs, ports, protocols, process names, label values, flow counts, dates. Those are the
evidence; without them the analysis is worthless.

The map file (anon-map.json) is the secret: keep it in the working folder (it is gitignored), never upload it.
`deanon` applies the inverse map to any CSV, so the umwl-import CSV that comes back from the analysis carries the
real hostnames/FQDNs in its descriptions before it is loaded into the PCE.
"""
import argparse, csv, ipaddress, json, os, re, sys

HOST_COLS = ["Source Name", "Source Hostname", "Source FQDN", "Destination Name", "Destination Hostname", "Destination FQDN"]
USER_COLS = ["Consuming Username", "Username"]
KEEP_USERS = {"root", "oracle", "zabbix", "nobody", "www-data", "apache", "nginx", "postgres", "mysql", "splunk", "daemon",
              "systemd-network", "systemd-resolve", "_apt", "sshd", "ntp", "chrony", "nagios", "tomcat", "weblogic", "grid",
              "network service", "local service", "system", "nt authority\\system", "nt authority\\network service"}
PRIVATE = [ipaddress.ip_network(n) for n in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16",
                                             "127.0.0.0/8", "224.0.0.0/4", "255.255.255.255/32", "0.0.0.0/8", "fc00::/7", "fe80::/10", "::1/128")]


def is_private(ip):
    try:
        a = ipaddress.ip_address(ip)
    except ValueError:
        return True
    return any(a in n for n in PRIVATE)


class Mapper:
    def __init__(self, path):
        self.path = path
        self.m = {"hosts": {}, "domains": {}, "users": {}, "ips": {}, "customer": {}}
        if os.path.exists(path):
            with open(path, encoding="utf-8") as f:
                self.m.update(json.load(f))
        self.dirty = False

    def save(self):
        if self.dirty:
            with open(self.path, "w", encoding="utf-8") as f:
                json.dump(self.m, f, indent=1, ensure_ascii=False)
            os.chmod(self.path, 0o600)

    def _next(self, table, key, fmt):
        t = self.m[table]
        if key in t:
            return t[key]
        t[key] = fmt(len(t) + 1)
        self.dirty = True
        return t[key]

    def domain(self, dom):
        dom = dom.lower().strip(".")
        if not dom:
            return ""
        parts = dom.split(".")
        # keep depth: acme.com → company.com ; unix.corp.acme.com → unix.dept.company.com (first label kept if it is a tier hint)
        if dom in self.m["domains"]:
            return self.m["domains"][dom]
        depth = len(parts)
        if depth <= 2:
            out = "company.com"
        elif depth == 3:
            out = "dept.company.com"
        else:
            out = parts[0] + ".dept.company.com"
        self.m["domains"][dom] = out
        self.dirty = True
        return out

    def host(self, value, anon_domains):
        v = value.strip()
        if not v or is_ip_literal(v):
            return v
        low = v.lower()
        if low in self.m["hosts"]:
            return self.m["hosts"][low]
        short, _, dom = low.partition(".")
        token = self._next("hosts", low, lambda n: f"host-{n:04d}")
        if dom:
            if anon_domains is None or any(dom == d or dom.endswith("." + d) for d in anon_domains):
                token = token + "." + self.domain(dom)
            else:
                token = token + "." + dom
            self.m["hosts"][low] = token
        return token

    def user(self, value):
        v = value.strip()
        if not v or v.lower() in KEEP_USERS:
            return v
        return self._next("users", v, lambda n: f"user-{n:02d}")

    def ip(self, value):
        v = value.strip()
        if not v or is_private(v):
            return v
        return self._next("ips", v, lambda n: f"203.0.113.{n}" if n < 255 else f"198.51.100.{n - 254}")

    def customer(self, text, names):
        for n in names:
            if not n:
                continue
            text = re.sub(r"(?i)\b" + re.escape(n) + r"\b", "Cliente", text)
            self.m["customer"][n] = "Cliente"
        return text

    def inverse(self):
        """Tokens that identify one real value: hosts, users, ips. Domains are many-to-one and are not inverted
        on their own (a host token already carries its real FQDN)."""
        inv = {}
        for table in ("hosts", "users", "ips"):
            for k, v in self.m[table].items():
                inv[v] = k
        return inv


def is_ip_literal(s):
    try:
        ipaddress.ip_address(s)
        return True
    except ValueError:
        return False


def anon(args):
    mp = Mapper(args.map)
    doms = [d.lower() for d in (args.domain or [])] or None
    with open(args.input, newline="", encoding="utf-8-sig") as f:
        rdr = csv.DictReader(f)
        rows = list(rdr)
        fields = rdr.fieldnames
    stats = {"hosts": 0, "users": 0, "ips": 0}
    for r in rows:
        for c in HOST_COLS:
            if c in r and r[c]:
                new = mp.host(r[c], doms)
                if new != r[c]:
                    stats["hosts"] += 1
                r[c] = new
        for c in USER_COLS:
            if c in r and r[c]:
                new = mp.user(r[c])
                if new != r[c]:
                    stats["users"] += 1
                r[c] = new
        if args.public_ips:
            for c in ("Source IP", "Destination IP"):
                if c in r and r[c]:
                    new = mp.ip(r[c])
                    if new != r[c]:
                        stats["ips"] += 1
                    r[c] = new
        if args.customer:
            for c in fields:
                if r.get(c):
                    r[c] = mp.customer(r[c], args.customer)
    with open(args.output, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        w.writerows(rows)
    mp.save()
    print(f"{len(rows)} rows → {args.output}; replaced hosts:{stats['hosts']} users:{stats['users']} public IPs:{stats['ips']}; map: {args.map} ({len(mp.m['hosts'])} hosts, {len(mp.m['domains'])} domains)")
    leak = check_leaks(args.output, mp, doms, args.customer or [])
    if leak:
        print("WARNING — possible leaks left in the output:\n  " + "\n  ".join(leak))


def check_leaks(path, mp, doms, customers):
    """Cheap second pass: real domains / customer names still present anywhere in the output."""
    text = open(path, encoding="utf-8").read().lower()
    found = []
    for d in (doms or []):
        if d in text:
            found.append(f"domain {d}")
    for n in customers:
        if n and n.lower() in text:
            found.append(f"customer name {n}")
    for real in mp.m["hosts"]:
        short = real.split(".")[0]
        if len(short) > 3 and re.search(r"\b" + re.escape(short) + r"\b", text):
            found.append(f"hostname {short}")
            if len(found) > 20:
                break
    return found


def deanon(args):
    mp = Mapper(args.map)
    inv = mp.inverse()
    # longest tokens first so host-0012.company.com is replaced before host-0012
    keys = sorted(inv, key=len, reverse=True)
    pat = re.compile("|".join(re.escape(k) for k in keys)) if keys else None
    with open(args.input, newline="", encoding="utf-8-sig") as f:
        rdr = csv.DictReader(f)
        rows = list(rdr)
        fields = rdr.fieldnames
    n = 0
    for r in rows:
        for c in fields:
            if r.get(c) and pat:
                new = pat.sub(lambda m: inv[m.group(0)], r[c])
                if new != r[c]:
                    n += 1
                r[c] = new
    with open(args.output, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        w.writerows(rows)
    print(f"{len(rows)} rows → {args.output}; {n} cells restored")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    a = sub.add_parser("anon", help="pseudonymize an export")
    a.add_argument("input")
    a.add_argument("-o", "--output", required=True)
    a.add_argument("--map", default="anon-map.json")
    a.add_argument("--customer", action="append", help="customer name(s) to replace with 'Cliente' everywhere (repeatable)")
    a.add_argument("--domain", action="append", help="domain(s) to pseudonymize; default: every domain seen")
    a.add_argument("--public-ips", action="store_true", help="also replace public IPs with 203.0.113.x / 198.51.100.x")
    a.set_defaults(fn=anon)
    d = sub.add_parser("deanon", help="restore real names in a CSV produced from the anonymized export")
    d.add_argument("input")
    d.add_argument("-o", "--output", required=True)
    d.add_argument("--map", default="anon-map.json")
    d.set_defaults(fn=deanon)
    args = ap.parse_args()
    if not os.path.exists(args.input):
        sys.exit(f"not found: {args.input}")
    args.fn(args)


if __name__ == "__main__":
    main()
