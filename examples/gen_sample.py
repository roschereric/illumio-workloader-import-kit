#!/usr/bin/env python3
"""Generate the neutral sample dataset (fictional lab, RFC 1918 addressing, plain Illumio label values).

Writes examples/sample-umwl-import.csv, sample-umwl-import-v2.csv, sample-ipl-import.csv, sample-ipl-import-v2.csv
and copies the v2 pair to testdata/.
"""
import csv, os, sys

ROOT = sys.argv[1]
G = "G1"  # analysis group tag used in descriptions: [G1 P<prio> conf:<Alta|Media|Baja>]

def row(name, ip, desc, role, app, env, loc, prio, conf, seen="v1+v2", review="PENDING"):
    d = f"[{G} P{prio} conf:{conf}] {desc} | Visto en: {seen}"
    return {"hostname": "", "name": f"{name} {ip}", "interfaces": f"eth0:{ip}", "description": d,
            "role": role, "app": app, "env": env, "loc": loc, "review": review, "_seen": seen, "_prio": prio}

W = []
# --- shared services (P1)
W.append(row("Zabbix Server", "10.10.4.21", "Servidor Zabbix: consulta 10050/TCP (chequeo pasivo) e ICMP en ord-app-01 y bil-app-01 (≈180.000 flujos en 7 días) y recibe 10051 (chequeo activo). Ya existe en el PCE como workload no gestionado: esta fila actualiza etiquetas", "Monitoring", "Monitoring", "Production", "DC1", 1, "Alta", review="UPDATED"))
W.append(row("Zabbix Proxy Cloud", "10.30.4.21", "Proxy Zabbix en la nube: 10050/ICMP hacia bil-app-01 (95.000 flujos), recibe 10051; también prueba 7776 (chequeo de servicio)", "Monitoring", "Monitoring", "Production", "Cloud", 1, "Alta"))
for ip in ("10.10.0.53", "10.10.0.54"):
    W.append(row("DNS Corporativo", ip, "Resolutor corporativo: 53/UDP y 53/TCP desde los tres VEN (≈40.000 consultas en 7 días). Existe en el PCE como 'DNS 1' (dos IPs en una sola interfaz): esta fila lo separa en un workload por IP", "DNS", "Shared Services", "Production", "DC1", 1, "Alta", review="UPDATED"))
W.append(row("NTP Corporativo", "10.10.0.123", "123/UDP desde ord-app-01 y bil-app-01 cada 64 s (proceso chronyd)", "NTP", "Shared Services", "Production", "DC1", 1, "Alta"))
W.append(row("SMTP Relay", "10.10.0.25", "25/TCP desde ord-app-01 (proceso java, ≈1.200 sesiones): correo de notificaciones de la aplicación", "MailRelay", "Shared Services", "Production", "DC1", 1, "Media"))
for ip in ("10.10.5.10", "10.10.5.11"):
    W.append(row("NetBackup", ip, "1556/TCP y 13724/TCP hacia los VEN (proceso bpcd) en ventana nocturna 01:00–03:00; el VEN abre 13782 de vuelta", "Backup", "Shared Services", "Production", "DC1", 1, "Alta"))
W.append(row("Splunk Indexer", "10.10.4.31", "Recibe 9997/TCP desde los tres VEN (splunkd, ≈3 GB en 7 días)", "LogCollector", "Monitoring", "Production", "DC1", 1, "Alta"))
W.append(row("Bastion SSH", "10.10.7.10", "Único origen de 22/TCP hacia los VEN (usuarios admin-01, admin-02; 340 sesiones)", "Bastion", "Admin Access", "Production", "DC1", 1, "Alta"))
W.append(row("Scanner Vulnerabilidades", "10.10.6.40", "Barrido de 1–65535/TCP contra los tres VEN el 2026-08-20 03:10–03:55 (hallazgo S-01); modelar como lista de IP con regla explícita", "Scanner", "Security Tools", "Production", "DC1", 1, "Alta"))
W.append(row("EDR Manager", "10.10.6.41", "Recibe 443/TCP y 4118/TCP desde el agente de los VEN cada 10 min (proceso ds_agent)", "AgentManager", "Security Tools", "Production", "DC1", 1, "Alta"))
# --- Ordering (DC1): 2 front-ends behind a VIP, VEN app server ord-app-01 10.20.2.20, DB
for ip in ("10.20.1.11", "10.20.1.12"):
    W.append(row("Front-End Ordering", ip, "Servidores web de Ordering: únicos consumidores de 8080/TCP en ord-app-01, volúmenes idénticos (≈600.000 flujos cada uno), misma /24. No son balanceadores: el VIP que los publica es 10.20.1.10", "Web", "Ordering", "Production", "DC1", 1, "Alta"))
W.append(row("VIP Ordering", "10.20.1.10", "VIP del balanceador que publica los front-ends de Ordering: recibe 443/TCP desde la VLAN de usuarios (≈2,1 M flujos) y reparte 80/TCP a 10.20.1.11/.12", "LoadBalancer", "Ordering", "Production", "DC1", 1, "Alta"))
W.append(row("Base de Datos Ordering", "10.20.3.30", "1521/TCP desde ord-app-01 (proceso java, usuario ordering; 1,4 M flujos, 22 GB). Sin VEN: candidata a la siguiente ola de agentes", "Database", "Ordering", "Production", "DC1", 1, "Alta"))
W.append(row("Cola de Mensajes Ordering", "10.20.3.40", "5672/TCP (AMQP) desde ord-app-01 y bil-app-01: integración entre las dos aplicaciones", "Messaging", "Ordering", "Production", "DC1", 1, "Media"))
# --- Billing (Cloud): 3 front-ends behind 2 VIPs (one per subnet), VEN bil-app-01 10.30.2.20, RAC
for ip in ("10.30.1.21", "10.30.1.22", "10.30.1.23"):
    W.append(row("Front-End Billing", ip, "Servidores web de Billing en la nube: consumen 7001/TCP en bil-app-01 con carga desigual por nodo (.21 concentra el 60 %); .22 y .23 además suben archivos por SFTP 22, algo que un balanceador no hace. Los balanceadores son los VIP 10.30.1.20 y 10.30.5.20", "Web", "Billing", "Production", "Cloud", 1, "Alta"))
for ip in ("10.30.1.20", "10.30.5.20"):
    W.append(row("VIP Billing", ip, "VIP del balanceador de Billing (uno por subred/zona): recibe 443/TCP desde Internet vía el WAF y reparte 8080/TCP a los front-ends", "LoadBalancer", "Billing", "Production", "Cloud", 1, "Alta"))
for ip in ("10.30.3.31", "10.30.3.32"):
    W.append(row("Base de Datos Billing RAC", ip, "1521/TCP desde bil-app-01 (usuario billing; ≈900.000 flujos); nodo RAC, interconnect entre .31 y .32 en 1521/UDP", "Database", "Billing", "Production", "Cloud", 1, "Alta"))
W.append(row("Servidor de Reportes Billing", "10.30.2.25", "Consume 7001/TCP en bil-app-01 fuera de horario (02:00–04:00) y 1521 en el RAC; genera reportes batch", "App", "Billing", "Production", "Cloud", 2, "Media"))
W.append(row("Destino SFTP Billing", "10.30.6.30", "22/TCP desde bil-app-01 (proceso sftp, 4.800 sesiones): intercambio de archivos con el ERP", "FileTransfer", "Billing", "Production", "Cloud", 2, "Media"))
# --- P2/P3
W.append(row("Servidor HTTP Legacy", "10.20.9.15", "80/TCP desde ord-app-01 (12.000 flujos, HTTP en claro; hallazgo S-04); aplicación a confirmar", "Web", "", "Production", "DC1", 2, "Baja"))
W.append(row("Consola OEM", "10.30.4.22", "Recibe 4903/TCP desde el RAC (agente OEM) y 3872 de vuelta", "Monitoring", "Monitoring", "Production", "Cloud", 2, "Alta"))
W.append(row("Zabbix Cloud 2", "10.30.4.23", "Segunda instancia Zabbix vista solo en los registros de flujo de la nube: recibe 10051 desde 10.40.1.x", "Monitoring", "Monitoring", "Production", "Cloud", 2, "Media", seen="v2"))
for ip, d in (("10.40.1.11", "Recibe 1521 desde 10.40.1.21 y .22; sale a Splunk 9997 y Zabbix 10051. No interactúa con los VEN (siguiente ola)"),
              ("10.40.1.12", "Recibe 1521 desde 10.40.1.21; recibe 123/UDP desde direcciones públicas (hallazgo S-06)")):
    W.append(row("Base de Datos Cloud", ip, d, "Database", "", "Production", "Cloud", 3, "Media", seen="v2"))
for ip, d in (("10.40.1.21", "Consume 1521 en 10.40.1.11/.12 y sirve 8001 a 10.40.1.22; sale a Zabbix 10051"),
              ("10.40.1.22", "Consume 8001 en 10.40.1.21 y 1521 en 10.40.1.11 (8001 denegado por el firewall de la nube desde 10.40.9.0/24)")):
    W.append(row("App Server Cloud", ip, d, "App", "", "Production", "Cloud", 3, "Media", seen="v2"))
for ip in ("10.20.9.40", "10.20.9.41"):
    W.append(row("Servicio a identificar", ip, "24000–24010/TCP consumidos por ord-app-01 (≈2.000 flujos) solo en v1; identificar el servicio antes de etiquetar", "", "", "Production", "DC1", 3, "Baja", seen="solo v1"))

IPL = [
    ("Ordering Front-End Subnet", f"[{G}] Subred de los front-ends y el VIP de Ordering (más el gateway 10.20.1.1) — uso: Ringfence Ordering", "10.20.1.0/24", "", ""),
    ("Billing Front-End Subnets", f"[{G}] Subredes de los front-ends y VIP de Billing (una por zona) — uso: Ringfence Billing", "10.30.1.0/24;10.30.5.0/24", "", ""),
    ("Corporate DNS", f"[{G}] Resolutores corporativos — uso: DNS", "10.10.0.53;10.10.0.54", "", ""),
    ("Corporate NTP", f"[{G}] NTP interno; excluye deliberadamente al público 203.0.113.10 (S-06) — uso: Tiempo", "10.10.0.123", "", ""),
    ("Monitoring Servers", f"[{G}] Zabbix (tres instancias), OEM y Splunk — uso: Monitoreo y logs", "10.10.4.21;10.30.4.21;10.30.4.23;10.30.4.22;10.10.4.31", "", ""),
    ("Backup Servers", f"[{G}] NetBackup — uso: Respaldo", "10.10.5.10;10.10.5.11", "", ""),
    ("Admin Access", f"[{G}] Origen SSH administrativo — uso: Administración", "10.10.7.10", "", ""),
    ("Vulnerability Scanners", f"[{G}] Escáner de vulnerabilidades (S-01) — uso: Escaneo autorizado", "10.10.6.40", "", ""),
    ("User VLANs", f"[{G}] VLAN de usuarios que consumen el VIP de Ordering — uso: Consumidores de Ordering", "10.100.0.0/16", "", ""),
    ("Cloud Metadata", f"[{G}] Metadata y resolver de la red virtual de la nube — uso: Plataforma cloud", "169.254.169.254", "", ""),
    ("Cloud Next Wave", f"[{G}] Subred de los cuatro hosts vistos solo en los registros de flujo de la nube (siguiente ola) — uso: Siguiente ola", "10.40.1.0/24", "", ""),
    ("Cloud Provider Services", f"[{G}] Servicios de plataforma del proveedor de nube; cargar los rangos publicados para la región — uso: Plataforma cloud", "", "", "*.objectstorage.example-cloud.com;*.monitoring.example-cloud.com"),
    ("EDR SaaS Console", f"[{G}] Consola SaaS del EDR; usar la lista publicada por el fabricante — uso: Agentes de seguridad", "", "", "*.edr.example-vendor.com"),
]

FIELDS = ["hostname", "name", "interfaces", "description", "role", "app", "env", "loc", "review"]
os.makedirs(os.path.join(ROOT, "examples"), exist_ok=True)
os.makedirs(os.path.join(ROOT, "testdata"), exist_ok=True)

def write_umwl(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=FIELDS, extrasaction="ignore")
        w.writeheader()
        w.writerows(rows)

def write_ipl(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["name", "description", "include", "exclude", "fqdns"])
        w.writerows(rows)

# v1: rows seen in v1, no "Visto en", review PENDING
v1 = []
for r in W:
    if r["_seen"] == "v2":
        continue
    r1 = dict(r)
    r1["description"] = r1["description"].split(" | Visto en:")[0]
    r1["review"] = "PENDING"
    v1.append(r1)
v2 = []
for r in W:
    r2 = dict(r)
    if r2["review"] == "PENDING" and r2["_seen"] == "v1+v2":
        r2["review"] = "UNCHANGED"
    v2.append(r2)

ex = os.path.join(ROOT, "examples")
write_umwl(os.path.join(ex, "sample-umwl-import.csv"), v1)
write_umwl(os.path.join(ex, "sample-umwl-import-v2.csv"), v2)
write_ipl(os.path.join(ex, "sample-ipl-import.csv"), [r for r in IPL if r[0] not in ("Cloud Next Wave",)])
write_ipl(os.path.join(ex, "sample-ipl-import-v2.csv"), IPL)
td = os.path.join(ROOT, "testdata")
write_umwl(os.path.join(td, "sample-umwl-import-v2.csv"), v2)
write_ipl(os.path.join(td, "sample-ipl-import-v2.csv"), IPL)
p1 = sum(1 for r in v2 if r["_prio"] == 1)
print(f"v1 rows {len(v1)}, v2 rows {len(v2)} (P1 {p1}), ipl v1 {len(IPL)-1}, ipl v2 {len(IPL)}")
