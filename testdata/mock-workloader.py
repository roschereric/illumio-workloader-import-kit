#!/usr/bin/env python3
# mock workloader for offline testing
import sys, csv, os, datetime
args=sys.argv[1:]
def opt(name):
    return args[args.index(name)+1] if name in args else None
log=opt("--log-file") or "workloader.log"
def L(msg): open(log,"a").write(f"{datetime.datetime.now():%Y-%m-%d %H:%M:%S} [INFO] - {msg}\r\n"); print(f"[INFO] - {msg}")
cmd=args[0] if args else ""
if cmd=="version": print("workloader v12.1.9 (mock)")
elif cmd=="pce-list": print("+------+------------------+\n| poc  | poc.illum.io     | default |")
elif cmd=="wkld-export":
    out=opt("--output-file"); rows=[["href","hostname","name","managed","interfaces","public_ip","role","app","env","loc","ven_href"],
        ["/orgs/1/workloads/a","atun4.ux.corp2.com","atun4","true","eth0:10.0.4.152/24","","R_NoRole","A_NoApp","E_NoEnv","L_NoLocation","/orgs/1/vens/1"],
        ["/orgs/1/workloads/b","zbx-old","Zabbix Server","false","umwl:10.43.43.21","","","","","",""],
        ["/orgs/1/workloads/c","dns1","DNS 1","false","umwl:192.168.161.105;umwl:192.168.161.92","","DNS","","","",""],
        ["/orgs/1/workloads/d","dns1-dup","DNS dup","false","umwl:192.168.161.105","","","","","",""]]
    if os.path.exists("created.txt"):
        for ip in open("created.txt").read().split(): rows.append([f"/orgs/1/workloads/{ip}",ip,ip,"false",f"umwl:{ip}","","","","","",""])
    csv.writer(open(out,"w",newline="")).writerows(rows); L(f"exported {len(rows)-1} workloads")
elif cmd=="label-export":
    csv.writer(open(opt("--output-file"),"w",newline="")).writerows([["href","key","value"],["/l/1","role","DNS"],["/l/2","env","Production"]])
elif cmd=="label-dimension-export":
    csv.writer(open(opt("--output-file"),"w",newline="")).writerows([["href","key","display_name"],["/d/1","role","Role"],["/d/2","app","Application"],["/d/3","env","Environment"],["/d/4","loc","Location"]])
elif cmd=="wkld-import":
    rows=list(csv.DictReader(open(args[1])))
    if "--update-pce" in args:
        if "--umwl" in args:
            open("created.txt","a").write(" ".join(r["interfaces"].split(":")[-1] for r in rows)+" ")
            L("created new role label - Monitoring - 201"); L(f"bulk create workload successful for {len(rows)} unmanaged workloads - status code 200")
        else: L(f"bulk update workload successful for {len(rows)} workloads - status code 200")
    else:
        for i,r in enumerate(rows,2): L(f"csv line {i} - {r.get('hostname')} to be created" if "--umwl" in args else f"csv line {i} - {r.get('hostname')} - {r.get('href')} - role to be changed from \"\" to \"{r.get('role')}\"")
        L(f"workloader identified 3 labels to create."); L(f"workloader identified {len(rows)} unmanaged workloads to create.")
elif cmd=="ipl-import":
    L("ipl-import mock: 14 ip lists to be created" if "--update-pce" not in args else "created 14 ip lists")
else: print("mock: unknown", cmd); sys.exit(1)
