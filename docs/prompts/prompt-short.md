# Short prompt — attach with the flow export

Paste this as the message, with the files attached. It assumes the project instructions (`context.md`) are loaded
in the Claude Project; if you are not in a Project, attach `context.md` as well and start with "Follow context.md".

---

Adjunto el export de Explorer/Traffic del PCE (`TrafficData….csv`) de la prueba de concepto, más `pce-workloads.csv`,
`pce-labels.csv` y `pce-label-types.csv` exportados con workloader. Los hostnames y FQDN están pseudonimizados
(host-NNNN.company.com); las IPs privadas son reales y son la clave.

Alcance: los workloads con VEN que aparecen en el export (identificalos vos: columna Enforcement con valor, hostname
presente). Grupo: `C4`. Nomenclatura de etiquetas del PCE: prefijos `R_`, `A_`, `E_`, `L_`; valores existentes en
`pce-labels.csv` (reusalos). Ubicaciones: `L_CDLV` (datacenter) y `L_OCI` (Oracle Cloud). Entorno: `E_Prod` salvo
evidencia en contra.

Entregables, siguiendo el método de las instrucciones del proyecto:
1. Informe HTML autocontenido en español, customer-facing, sin nombre del cliente, con las 12 secciones y los
   diagramas SVG (composición del export, criterio workload vs lista de IP, arquitectura en capas por aplicación,
   construcción de la fila del CSV).
2. `C4-umwl-import.csv` (una fila por IP, hostname vacío, name = rol + IP, interfaces eth0:<ip>, descripción con
   evidencia y prioridad, etiquetas R_/A_/E_/L_, review=PENDING).
3. `C4-ipl-import.csv` (name,description,include,exclude,fqdns).
4. Un resumen en el chat de 10 líneas: ventana, truncamiento, hosts con VEN, cuántos workloads/listas propone, los
   3 hallazgos más importantes y qué necesitás que confirme yo.

Antes de responder verificá contra documentación oficial cualquier puerto de producto, versión o fecha de fin de
soporte que menciones, y aplicá la puerta de calidad de las instrucciones (conteos consistentes, sin R_LoadBalancer en
servidores que no sean VIPs, sin roles derivados de hostnames).

---

## Variant — update with a new export (v2)

Adjunto un export nuevo del mismo alcance, el informe anterior y los CSV ya propuestos/cargados. Necesito la v2:
qué cambió (pares nuevos, pares que no aparecen, decisiones de política que pasaron a Allowed, etiquetas que el PCE
muestra distinto), los mismos IDs de hallazgos con su estado, y los dos CSV completos otra vez con `review` =
PENDING / UPDATED / UNCHANGED.
