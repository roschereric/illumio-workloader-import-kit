English: [README.md](README.md)

# illumio-workloader-import-kit

Kit para cargar en un PCE de Illumio los **workloads no gestionados (UMWL)** y las **listas de IP** que salen de un análisis de flujos de Explorer, usando [workloader](https://github.com/brian1917/workloader) como motor de importación. Incluye `umwl-tui`, una aplicación de terminal de pantalla completa (Go) que reconcilia por IP contra el inventario del PCE antes de escribir nada, su predecesora en Python de terminal plana, un script mínimo no interactivo, los CSV de ejemplo de una prueba de concepto y una guía explicativa en PDF con diagramas (en español y en inglés).

> Verificado contra **workloader v12.1.9** (release del 12 de junio de 2025; código en `master` consultado el 3 de septiembre de 2026) e **Illumio Core 24.x/25.x** (documentación en `product-docs-repo.illumio.com`). Los nombres de comandos, flags y cabeceras CSV de este README se tomaron del código fuente de workloader (`cmd/root.go`, `cmd/wkldimport`, `cmd/iplimport`, `cmd/wkldexport`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension`, `cmd/traffic`). Si usás otra versión, confirmá con `./workloader <comando> --help`.

---

## Importante: una carpeta de trabajo por cuenta (organización) de Illumio + PCE

workloader resuelve la conexión al PCE leyendo, en este orden, `--config-file`, la variable `ILLUMIO_CONFIG` y, si no hay ninguna, **`./pce.yaml` en la carpeta de trabajo**. El kit, además, escribe en la carpeta de trabajo `runs/<timestamp>/` con el inventario exportado del PCE, los CSV finales y los logs de cada lote.

La unidad de aislamiento no es "el cliente" ni "el PCE" sino cada combinación **cuenta u organización de Illumio + PCE**, es decir, cada perfil distinto de `pce.yaml` (FQDN, puerto, org id y API key):

- Una cuenta puede tener varios PCE (SaaS y on-prem, producción y DR, regiones): cada uno es una carpeta.
- Un PCE SaaS (un FQDN) aloja varias organizaciones; cada `org id` es un tenant distinto y lleva su propia carpeta, aunque el FQDN sea el mismo. El `org id` es el número que aparece en la URL de la consola (`…/orgs/<id>/…`).
- Otra cuenta: misma estructura, nunca el mismo `pce.yaml`.

Si en una misma carpeta conviven `pce.yaml` de dos tenants (o un `pce.yaml` con varios PCE y te olvidás del `--pce`), el riesgo es concreto: **importar los objetos de un cliente en el PCE o la organización de otro**. La regla es simple: **una carpeta de trabajo por cuenta + PCE**, con su propio `pce.yaml`, sus CSV y sus `runs/`. Nomenclatura sugerida: `~/illumio/<cuenta>/<pce-o-org>/`.

Estructura sugerida:

```
~/illumio/
├── cliente-a/
│   ├── scp57-org12/                  # cuenta cliente-a, PCE SaaS scp57, org 12
│   │   ├── kit/                      # clon de este repositorio
│   │   ├── workloader                # binario (o symlink a uno compartido)
│   │   ├── pce.yaml                  # SOLO esta cuenta + PCE
│   │   ├── cliente-a-umwl-import.csv
│   │   ├── cliente-a-ipl-import.csv
│   │   └── runs/
│   │       └── 20260903-101500/
│   └── onprem-prod/                  # misma cuenta, otro PCE: otra carpeta, otro pce.yaml
│       ├── kit/  workloader  pce.yaml  ...
└── cliente-b/
    └── <pce-o-org>/                  # otra cuenta, misma estructura
        ├── kit/  workloader  pce.yaml  ...
```

Antes de cada corrida, la TUI ejecuta `workloader pce-list` y te muestra el PCE que va a usar; leé ese nombre y el FQDN antes de aceptar. Si el `pce.yaml` tiene más de un PCE, pasá siempre `--pce <nombre>`. workloader admite varios perfiles en un solo `pce.yaml` (con `default_pce_name` y el flag `--pce`), pero el kit no se apoya en eso a propósito: un `--pce` olvidado o un default equivocado cargaría los objetos en el tenant incorrecto.

---

## Contenido del repositorio

| Archivo | Qué es |
|---|---|
| `umwl-tui` (`cmd/`, `internal/`, `Makefile`) | **La aplicación de pantalla completa (Go, Bubble Tea)**: barra de estado, lista de pasos, panel por paso, salida de workloader en vivo y barra de teclas; modales para cada decisión; lotes con progreso y reintento/salto; reporte por corrida. Un solo binario estático para macOS (arm64/amd64) y Linux. Ver la sección de abajo y `docs/SPEC-umwl-tui.md`. |
| `umwl_loader.py` | Versión de terminal plana (Python 3, solo biblioteca estándar) del mismo flujo, mantenida como alternativa. Instala o verifica workloader, configura el PCE, valida el CSV, reconcilia por IP, hace dry run, importa por lotes con barra de progreso, verifica y deja un reporte. |
| `reconcile_umwl.py` | Versión mínima y no interactiva del paso de reconciliación por IP: a partir de un `wkld-export` y del CSV propuesto genera `-to-create.csv`, `-existing.csv`, `-conflicts.csv` y un reporte de texto. No escribe en el PCE. |
| `examples/*-umwl-import.csv` | CSV de workloads no gestionados propuestos (una fila por IP) de tres grupos de una prueba de concepto. Son **ejemplos de POC**: contienen IPs de laboratorio del cliente y sirven como plantilla de formato, no para cargar tal cual. |
| `examples/cliente3-umwl-import-v2.csv` | **Formato recomendado** a partir de ahora: etiquetas con prefijo `R_/A_/E_/L_`, `name` = rol descriptivo + IP, `hostname` vacío, `interfaces` = `eth0:<ip>`, comentario del informe en `description`. |
| `examples/*-ipl-import.csv` | Listas de IP propuestas en el formato de `workloader ipl-import`. `cliente3-ipl-import-v2.csv` usa la convención `IPL_<Sitio>_<Uso>`. |
| `docs/Guia-workloader-import-kit.pdf` | Guía explicativa en español con diagramas (flujo completo, requisitos y una carpeta por cuenta + PCE, inicialización, pasos de la TUI, relación kit/workloader, mapeo CSV → objetos del PCE, solución de problemas). También en `docs/Guia-workloader-import-kit.html` (archivo único, se abre con doble clic). |
| `docs/Guide-workloader-import-kit-EN.pdf` | La misma guía en inglés. También en `docs/Guide-workloader-import-kit-EN.html`. |
| `.gitignore` | Excluye `workloader` (binario), `workloader/` (clon), `pce.yaml`, `*.log`, `runs/`, zips. **Nunca subas `pce.yaml`: contiene la API key.** |

Cambios respecto de la versión original del kit:

- En `umwl_loader.py` los dos caminos de configuración del PCE (`[k]` ingresar los datos en la TUI, `[i]` usar los prompts de workloader) llaman a `pce-add --api-key`, así workloader pide API Authentication Username / API Secret / Org y nunca correo y contraseña. Sin ese flag, workloader ignora `--api-user/--api-secret/--org` y pide correo y contraseña (verificado en `cmd/pcemgmt/addpce.go`).
- La TUI detecta el mensaje "no pce configured" que imprime `workloader pce-list` (con código de salida 0) cuando el `pce.yaml` existe pero no tiene entradas, y va directo a `pce-add` en lugar de preguntar si ese es el PCE correcto.

---

## Requisitos

- **macOS** (probado en Apple Silicon; el flujo es el mismo en Intel y en Linux).
- **Python 3.9 o superior**, solo biblioteca estándar (no hay `pip install`).
- **workloader v12.x**. No requiere instalación: es un binario. El release de macOS se publica como `mac-<versión>.zip`; en Apple Silicon corre vía Rosetta 2. Si preferís un binario nativo, **Go** (`brew install go`) y `go build` desde el clon del repositorio; la TUI ofrece ambas opciones.
- Una **API key del PCE con permisos de escritura**. La key hereda el rol del usuario que la crea: si el usuario es de solo lectura, `wkld-import --update-pce` falla. Para crearla: menú de usuario (arriba a la derecha) → **My API Keys** → **Add**; guardá el **Authentication Username** (`api_…`) y el **Secret**, que solo se muestran una vez. También sirve una service account con permisos equivalentes.
- **Acceso de red al PCE por HTTPS** (puerto 443 en SaaS, 8443 típico on-prem) desde la Mac. En SaaS el `org id` es el número que aparece en la URL de la consola (`…/orgs/<id>/…`).

---

## Paso a paso: extraer los flujos desde Explorer

El insumo del análisis es el CSV de tráfico del PCE. El nombre de la vista cambió entre generaciones de consola: en las consolas 22.x–23.x se llama **Explorer**; en las consolas 24.x/25.x los mismos datos están en la vista **Traffic** (tabla) dentro de la categoría **Explore** del menú izquierdo, junto a **Map**. La mecánica es la misma:

1. Abrí la consola del PCE y entrá a la vista **Explorer / Traffic**.
2. **Ventana de tiempo**: elegí un rango (último día, semana, mes o un rango personalizado). Para inferir roles con confianza conviene una ventana de al menos 7 días que incluya un fin de semana y, si existen, ventanas de respaldo y de escaneo.
3. **Filtros**: en *Source* (consumer) y *Destination* (provider) podés incluir o excluir workloads, IPs y etiquetas; en *Service*, puertos, protocolos y procesos. Para una prueba de concepto lo habitual es incluir en Source **o** Destination los workloads con VEN (o su etiqueta de aplicación) y dejar el otro lado abierto, con el operador "or" entre ambos lados para capturar entrada y salida.
4. **Límite de resultados**: el número máximo de filas del export lo fija la consulta. La consola muestra hasta 10.000 conexiones por página y hasta 100.000 resultados en pantalla; el CSV descargado puede llegar a 200.000 en un PCE standalone. **El export que analizamos venía con exactamente 5.000 filas**, que era el tope configurado en esa consulta, y un solo escaneo de puertos ocupaba el 93 % de ellas. Antes de exportar subí el límite al máximo que permita la consola y, si aun así se llena, **dividí la consulta por aplicación/grupo de workloads y por ventanas de tiempo más cortas**, y excluí el origen del escáner. Un export exactamente en el tope es un export truncado.
5. Ejecutá la consulta (**Run**). Si la consulta es asíncrona, aparece después en **Load Results** (arriba a la derecha); desde ahí se abre y se exporta.
6. **Export** → CSV. Guardá el archivo con un nombre que incluya cliente, alcance y fechas (por ejemplo `cliente-a_grupo1_2026-08-17_2026-09-01.csv`).
7. **Abrí el CSV como texto** (editor, `head`, Python, pandas). Si lo abrís en Excel, importalo con el asistente marcando las columnas de IP como **Texto**: Excel interpreta `10.0.4.10` como número o fecha y reconstruir las IPs después es trabajo perdido. Lo mismo con las columnas de fecha: en exports previos llegaron mezclados dos formatos.

Columnas del export (consola 24.x/25.x; una columna por tipo de etiqueta): `Source IP`, `Source Name`, `Source Hostname`, `Source Enforcement`, `Source App/Env/Loc/Role` (y cualquier tipo adicional), `Source FQDN`, las mismas para `Destination *`, `Port`, `Protocol`, `Process`, `Username`, `Num Flows`, `Bytes In`, `Bytes Out`, `Connection State`, `Reported Policy Decision`, `Reported by`, `First Detected`, `Last Detected`. Las columnas de etiquetas de origen y destino son las que permiten separar tráfico entre workloads gestionados del tráfico hacia IPs sin VEN.

### Alternativas al export manual

- **API REST asíncrona** (la misma que usa la consola): `POST /api/v2/orgs/<org>/traffic_flows/async_queries` con `query_name`, `sources`, `destinations`, `services`, `policy_decisions`, `start_date`, `end_date` y `max_results` (límite 200.000); después `GET …/traffic_flows/async_queries/<uuid>` para el estado y `GET …/traffic_flows_async/queries/<uuid>/download` para el resultado. El endpoint síncrono `traffic_flows/traffic_analysis_queries` está obsoleto.
- **workloader**: el comando actual es `workloader traffic` ("Export traffic data"). Flags relevantes: `--start` / `--end` (`yyyy-mm-dd` o `yyyy-mm-ddTHH:mm:ss`, en GMT; por defecto 88 días atrás y mañana), `--max-results` (por defecto 100.000, máximo 200.000), `--incl-src-file` / `--excl-src-file` / `--incl-dst-file` / `--excl-dst-file` (archivos con hrefs de etiquetas, listas de IP o workloads, que se obtienen con `label-export`, `ipl-export`, `wkld-export`), `--incl-svc-file` / `--excl-svc-file`, `--excl-allowed` / `--excl-potentially-blocked` / `--excl-blocked`, `--output-file`. El comando `explorer` sigue registrado pero como paquete `legacy-explorer`; usá `traffic`.

  ```bash
  ./workloader traffic --start 2026-08-17 --end 2026-09-01 --max-results 200000 --output-file flujos.csv
  ```

  Las columnas del CSV de `workloader traffic` no son idénticas a las del export de la consola; el prompt de análisis (más abajo) pide normalizar nombres de columna, así que cualquiera de los dos sirve.

---

## Objetos de política: qué crea el kit y por qué

- **Workloads no gestionados (UMWL).** Entidades de red sin VEN que se dan de alta en el PCE para poder escribir reglas sobre ellas; la política entre un workload con VEN y uno no gestionado se aplica con las reglas del lado que tiene VEN. El criterio del análisis: **un peer se modela como UMWL cuando es un servidor concreto con rol identificable** (resolutores DNS, NTP internos, servidor y proxies Zabbix, indexadores Splunk, NetBackup master/media, bases de datos, balanceadores o front-ends, bastiones, escáneres de vulnerabilidades, relays SMTP, IBM MQ, OEM). Se crea **uno por IP**, con etiquetas, y aparece en las reglas de ringfence como cualquier otro workload. En la consola equivale a *Workloads → Add → Add Unmanaged Workload*.
- **Listas de IP (IP lists).** Colecciones estáticas de direcciones, rangos y FQDN. Se usan para **peers amplios o que no son servidores**: VLAN de usuarios, subredes completas de una aplicación, metadata de nube (`169.254.169.254`, que en OCI es además el resolver de la VCN), NTP público, consolas SaaS de EDR, rangos publicados por un proveedor. También conviven con los UMWL como atajo: una lista `ipl-oracle-db` permite escribir hoy la regla "app → Oracle 1521" y migrarla a etiquetas después.
- **Etiquetas y tipos de etiqueta.** Los cuatro tipos por defecto son Role, Application, Environment y Location (RAEL); el PCE admite tipos adicionales (por ejemplo `os`). Al escribir reglas, Illumio aplica OR entre valores del mismo tipo y AND entre tipos distintos. **workloader crea valores de etiqueta que no existen, pero no crea tipos**: si el CSV trae una columna que no es un tipo del PCE, `wkld-import` la ignora en silencio (la TUI lo detecta en el paso 3 y ofrece descartarla o salir para crear el tipo con `label-dimension-import`).
- **Servicios.** Objetos de puerto/protocolo (`svc-dns`, `svc-app-8080`). El informe los propone, pero **el kit no los crea**; se cargan con `workloader svc-import` o desde la consola cuando se escriben las reglas.

### Convención de nomenclatura

| Campo | Convención | Ejemplo |
|---|---|---|
| Etiquetas | Prefijo por tipo: `R_<Rol>`, `A_<App>`, `E_<Entorno>`, `L_<Ubicación>` | `R_DNS`, `A_CoreInfra`, `E_Prod`, `L_OCI` |
| `name` | `<rol descriptivo> <IP>`; es lo que se ve en la consola y lo que hace única a cada fila | `Zabbix Server 10.43.43.21` |
| `hostname` | **Vacío**. Los FQDN de los exports pueden estar anonimizados o ser inventados; un hostname falso confunde más de lo que ayuda. workloader identifica la fila por `name` cuando no hay hostname. | |
| `interfaces` | `eth0:<ip>` (la consola lo muestra como *eth0: 10.1.1.1*); varias interfaces separadas por `;` | `eth0:10.43.43.21` |
| `description` | Comentario del análisis con el prefijo `[<grupo> P<prioridad> conf:<Alta\|Media\|Baja>]`; la prioridad es la que filtra `--priority` | `[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP…` |
| `review` | Columna de trabajo (PENDING, NEW, EXISTS-UNMANAGED…); workloader la ignora | |

---

## Contrato CSV de workloader

### `wkld-import` (workloads)

Cabeceras reconocidas (el resto se ignora): `href`, `hostname`, `name`, `interfaces`, `public_ip`, `distinguished_name`, `spn`, `enforcement`, `visibility`, `description`, `os_id`, `os_detail`, `data_center`, `external_data_set`, `external_data_reference` y **una columna por tipo de etiqueta** (`role`, `app`, `env`, `loc`, y los tipos personalizados que existan en el PCE).

```
hostname,name,interfaces,description,role,app,env,loc,review
,Zabbix Server 10.43.43.21,eth0:10.43.43.21,[C3 P1 conf:Alta] Servidor Zabbix…,R_Monitoring,A_Observability,E_Prod,L_CDLV,PENDING
```

- `interfaces`: `192.168.200.20`, `192.168.200.20/24`, `eth0:192.168.200.20` o `eth0:192.168.200.20/24`, separados por `;`.
- **Matching**: workloader busca el workload existente por `href` si viene, si no por `hostname`, si no por `name` (`--match` permite forzar `href|hostname|name|external_data`). **Nunca por IP.** Por eso dos filas con el mismo `name` y sin hostname se tratan como el mismo workload (la TUI les agrega la IP como sufijo), y por eso existe la reconciliación por IP: sin ella, un UMWL que ya existe con otro nombre se crea duplicado.
- `--umwl`: crea workloads no gestionados cuando el host no existe (se desactiva si se hace match por href). `--update` (por defecto `true`): actualiza los existentes; `--update=false` solo crea. `--allow-enforcement-changes`: necesario para tocar `enforcement`/`visibility`. `--max-create` / `--max-update`: tope de seguridad (-1 = sin límite).
- Sin `--update-pce`, el comando es un **dry run**: no escribe nada y deja en `workloader.log` (o en `--log-file`) qué crearía y qué cambiaría. Con `--update-pce` pide confirmación; con `--update-pce --no-prompt` no la pide (automatización).

### `ipl-import` (listas de IP)

```
name,description,include,exclude,fqdns
ipl-dns-corp,Resolutores corporativos,192.168.161.92;192.168.161.104;192.168.161.105,,
ipl-oci-metadata,Resolver de la VCN y metadata de OCI,169.254.169.254/32,,
```

- `include` / `exclude`: IPs, CIDR o rangos separados por `;`. `fqdns` para nombres.
- Matching por `href` y, si no hay, por `name`: si el nombre existe, actualiza; si no, crea. `--ignore-href` para reutilizar un export de otro PCE. `--provision` (`-p`) provisiona después de crear/actualizar.
- Igual que arriba: sin `--update-pce` es dry run.

---

## Inicialización de la carpeta de trabajo

Estos pasos se hacen una vez por combinación cuenta + PCE. Dejan una carpeta con el clon del kit, el binario de workloader y un `pce.yaml` probado; después, cada carga es un solo comando. El repositorio del kit es privado: hace falta una sesión de GitHub (`gh auth login` o una clave SSH cargada en la cuenta) antes de clonar.

### a. Carpeta de trabajo para esta cuenta + PCE

```bash
mkdir -p ~/illumio/cliente-a/scp57-org12 && cd ~/illumio/cliente-a/scp57-org12
```

La carpeta es la unidad de aislamiento; todo lo que sigue se ejecuta con esa carpeta como directorio actual.

### b. El kit dentro de la carpeta (repositorio privado)

```bash
gh auth login                                   # una vez; o una clave SSH cargada en GitHub
git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit
# equivalente: gh repo clone roschereric/illumio-workloader-import-kit kit
# sin git: en GitHub, Code → Download ZIP, y descomprimir como kit/
```

Resultado esperado. Los scripts se ejecutan desde la carpeta de trabajo como `python3 kit/umwl_loader.py …`: el directorio actual es la carpeta de trabajo, así que `./workloader`, `./pce.yaml` y `./runs` quedan junto a los CSV y no dentro del clon. Los CSV viven en la carpeta de trabajo, no en `kit/`; `kit/examples/` contiene solo plantillas. Ejecutar dentro del propio clon también funciona (su `.gitignore` excluye `pce.yaml`, el binario y `runs/`), a costa de atar ese clon a una sola cuenta + PCE.

```
~/illumio/cliente-a/scp57-org12/
  kit/                      # clon del repo; actualizar con: git -C kit pull
  workloader                # binario; lo descarga la TUI (o symlink a uno compartido)
  pce.yaml                  # SOLO esta cuenta + PCE; lo crea pce-add; nunca copiarlo
  cliente-a-umwl-import.csv
  cliente-a-ipl-import.csv
  runs/                     # una subcarpeta por corrida
```

### c. workloader y pce.yaml con la TUI

(Con la aplicación en Go: `./umwl-tui --setup-only` hace el mismo paso 0 — ver la sección umwl-tui.)

```bash
python3 kit/umwl_loader.py --setup-only
```

La TUI busca `./workloader` (después el PATH, o la ruta de `--workloader`). Si no lo encuentra ofrece descargar el release (`mac-<versión>.zip`, con `curl` + `unzip` y `xattr -d com.apple.quarantine`) o clonar el repositorio de workloader en `workloader-src/` y compilar con `go build`. Después corre `pce-list`; si no hay `pce.yaml`, o workloader responde "no pce configured", va directo a `pce-add`: pide nombre corto, FQDN, puerto, API user, API secret y org id, ejecuta `pce-add --api-key …` con esos valores, vuelve a mostrar `pce-list` y prueba la conexión con un `label-dimension-export`.

### d. Prueba de cordura (solo lectura)

```bash
./workloader pce-list
./workloader wkld-export --output-file sanity.csv
# mirar los hostnames de sanity.csv: ¿es el inventario del tenant esperado?
```

Antes de escribir nada, un export de solo lectura demuestra que la API key funciona y que el inventario es el del tenant esperado.

### e. CSV revisados y primera carga

```bash
cp ~/Downloads/cliente-a-umwl-import.csv ~/Downloads/cliente-a-ipl-import.csv .
python3 kit/umwl_loader.py cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1
```

Los CSV del informe, ya revisados, van a la carpeta de trabajo con el nombre del cliente. La TUI recorre los pasos 0 a 10 (ver "Cómo se usa"); escribe en el PCE solo después de la confirmación del dry run.

| Situación | Qué hacer |
|---|---|
| f. Otra cuenta u otro PCE | Repetir a–d en una carpeta nueva. Nunca copiar `pce.yaml`. Si se prefiere un solo clon del kit, compartirlo con un symlink: `ln -s ~/illumio/kit-shared kit`. |
| g. Actualizar el kit | `git -C kit pull` (o volver a descargar el zip). El loader no tiene otras dependencias: no hay nada más que instalar. |
| Varios PCE en un mismo `pce.yaml` | No es el esquema del kit. Si igual existe, pasar siempre `--pce <nombre>` a `umwl_loader.py`; la TUI muestra `pce-list` y pregunta si es el PCE correcto antes de seguir. |

---

## umwl-tui: la aplicación de pantalla completa

`umwl-tui` es el mismo flujo de diez pasos de `umwl_loader.py`, reconstruido como una aplicación de terminal real (estilo htop / Midnight Commander): barra de estado con el PCE, la carpeta de la corrida y el contador de pasos; lista de pasos con su estado a la izquierda; panel del paso activo a la derecha (tablas que se recorren con las flechas, detalle "propuesto vs. en el PCE" lado a lado, barras de progreso); salida de workloader en vivo abajo; y una barra de teclas que cambia por paso. Cada decisión que importa (conflictos, escrituras al PCE, lotes fallidos) es un modal; no se escribe nada antes de la confirmación "Write to the PCE?" posterior al dry run. La interfaz está en inglés.

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

### Instalación

Los binarios precompilados van adjuntos a los releases de GitHub (`umwl-tui-darwin-arm64`, `umwl-tui-darwin-amd64`, `umwl-tui-linux-amd64`, `umwl-tui-linux-arm64`, más `SHA256SUMS`). En una Mac:

```bash
cd ~/illumio/<cuenta>/<pce>                        # la carpeta de trabajo (ver "Inicialización de la carpeta de trabajo")
gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui
```

O compilarlo (Go 1.24+): `git clone … kit && (cd kit && make build) && cp kit/umwl-tui .`. `make dist` compila todas las plataformas en `dist/` con checksums.

### Uso

```bash
./umwl-tui --setup-only                                   # solo el paso 0: workloader, pce.yaml (pce-add --api-key), prueba de conexión
./umwl-tui cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1
./umwl-tui --help                                         # --pce, --workloader, --chunk (20 por defecto), --runs (./runs por defecto)
```

Teclas: `tab` pasa el foco al panel de log (se recorre con las flechas), `?` ayuda, `q` salir (pregunta una vez que empezó el trabajo). En la reconciliación: `u` actualizar el objeto existente, `s` omitir, `c` crear igual, `r` renombrar y actualizar, `U`/`S` aplicar a todas las filas del mismo tipo, `n` siguiente fila sin decidir. En la revisión: `e` editar name/hostname/description/etiquetas, `s` omitir. Un lote fallido abre un modal con reintentar / saltar / abortar.

Todo lo que produjo la corrida queda en `runs/<timestamp>/`: inventarios del PCE antes y después, `to-create.csv`, `to-update.csv`, `skipped.csv`, un CSV y un log de workloader por lote, `report.md` y `report.json` (con cada comando de workloader y su código de salida).

Notas de diseño y desarrollo: `docs/SPEC-umwl-tui.md` (máquina de estados, contrato de UI, contrato con workloader, requisitos y pruebas de seguridad, backlog) y `CLAUDE.md` (comandos de build/test/release y convenciones para sesiones de Claude Code).

## Cómo se usa

### 1. Preparar la carpeta de trabajo

Siempre desde la carpeta de trabajo de la cuenta + PCE (ver "Inicialización de la carpeta de trabajo"):

```bash
cd ~/illumio/cliente-a/scp57-org12
# copiá acá los CSV propuestos para ESTE tenant (no dentro de kit/)
python3 kit/umwl_loader.py --setup-only
```

`--setup-only` busca `./workloader` (o `--workloader <ruta>`, o el PATH). Si no lo encuentra ofrece descargar el último release (`curl` + `unzip`, quita la cuarentena de Gatekeeper) o clonar y compilar con Go. Después corre `pce-list`; si no hay `pce.yaml` (o workloader responde "no pce configured") guía el `pce-add` con la API key y prueba la conexión con un `label-dimension-export`.

### 2. Cargar

```bash
python3 kit/umwl_loader.py cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1
```

Flags de `umwl_loader.py`:

| Flag | Significado |
|---|---|
| `csv` | CSV de workloads propuestos, una fila por IP (formato `wkld-import`). |
| `--setup-only` | Solo instalar/verificar workloader y la conexión al PCE; no carga nada. |
| `--ipl <csv>` | CSV de listas de IP (formato `ipl-import`) para el paso 9. |
| `--pce <nombre>` | Nombre del PCE en `pce.yaml` (se pasa como `--pce` a workloader). Obligatorio si `pce.yaml` tiene más de un PCE (no es el esquema recomendado: una carpeta por cuenta + PCE). |
| `--priority 1` o `1,2` | Cargar solo las filas cuyo `description` empiece con `[.. P<n> ..]` para esas prioridades. |
| `--workloader <ruta>` | Ruta al binario (por defecto `./workloader`, después el PATH). |
| `--chunk 20` | Filas por llamada a `wkld-import`. Granularidad del progreso y radio de impacto de un error. |
| `--runs ./runs` | Carpeta donde se crea `runs/<timestamp>/`. |

Qué hace cada paso de la TUI:

| Paso | Qué hace | Toca el PCE |
|---|---|---|
| 0 Preflight | Localiza workloader, muestra el PCE (`pce-list`), crea `runs/<ts>/`. | No |
| 1 Validar CSV | Columnas obligatorias (`hostname`, `name`, `interfaces`), IPs válidas, IPs repetidas, filtro `--priority`, hostnames o names duplicados (les agrega la IP como sufijo), resumen de valores por etiqueta. | No |
| 2 Inventario | `wkld-export`, `label-export`, `label-dimension-export` a `runs/<ts>/`. Indexa cada workload del PCE por sus IPs (`interfaces` y `public_ip`) y marca los gestionados (`managed` o `ven_href`). | Solo lectura |
| 3 Etiquetas | Columnas del CSV que no son un tipo de etiqueta del PCE → descartar o salir a crear el tipo. Lista los valores nuevos que se crearán y pide confirmación. | No |
| 4 Reconciliar por IP | Cada fila se clasifica: **NEW** (la IP no existe), **EXISTS-UNMANAGED** (la IP pertenece a un UMWL: por defecto se actualizan etiquetas/descripción del existente, por `href`), **CONFLICT-MANAGED** (la IP pertenece a un workload con VEN: por defecto se omite), **CONFLICT-MULTIPLE** (varios workloads comparten la IP). Para cada existente/conflicto: actualizar, omitir, crear igual, renombrar, o aplicar la decisión a todos los del mismo tipo. | No |
| 5 Revisar NEW | Tabla de los nuevos; aceptar todos o revisar uno por uno editando name/hostname/description/etiquetas. Escribe `to-create.csv`, `to-update.csv`, `skipped.csv`. | No |
| 6 Dry run | `wkld-import to-create.csv --umwl --update=false` y `wkld-import to-update.csv`, sin `--update-pce`; muestra las líneas relevantes del log de workloader. Pide confirmación explícita antes de seguir. | No |
| 7 Ejecutar | Divide en lotes de `--chunk` filas y corre `wkld-import … --update-pce --no-prompt` por lote con barra de progreso; ante error: reintentar, saltar el lote o abortar. | **Sí** |
| 8 Verificar | Nuevo `wkld-export` y confirma que cada IP creada aparece como workload. | Solo lectura |
| 9 Listas de IP | Con `--ipl`: dry run de `ipl-import`, confirmación, `ipl-import … --update-pce --no-prompt`. | **Sí** |
| 10 Reporte | `runs/<ts>/report.md` y `report.json`: creados, actualizados, omitidos, lotes fallidos, etiquetas creadas, verificación y todos los comandos ejecutados con su código de salida. | No |

### 3. Sin la TUI (comandos workloader equivalentes)

Misma carpeta de trabajo, mismo `pce.yaml`:

```bash
./workloader pce-add --api-key --name cliente-a --fqdn pce.cliente-a.example --port 443 \
    --api-user api_xxxxxxxx --api-secret '…' --org 1 --disable-tls-verification false
./workloader pce-list
./workloader wkld-export --output-file pce-workloads.csv           # inventario
python3 kit/reconcile_umwl.py pce-workloads.csv cliente-a-umwl-import.csv
./workloader wkld-import cliente-a-umwl-import-to-create.csv --umwl  # dry run -> workloader.log
./workloader wkld-import cliente-a-umwl-import-to-create.csv --umwl --update-pce
./workloader wkld-import cliente-a-umwl-import-existing.csv          # solo etiquetas/descr., match por href
./workloader ipl-import cliente-a-ipl-import.csv                      # dry run
./workloader ipl-import cliente-a-ipl-import.csv --update-pce
```

`reconcile_umwl.py` deja junto al CSV propuesto: `-to-create.csv` (IPs que no existen), `-existing.csv` (IPs de UMWL existentes, con `href` y `hostname` reescritos para que `wkld-import` sin `--umwl` solo actualice etiquetas y descripción), `-conflicts.csv` (IPs de workloads con VEN o compartidas: decidir a mano) y `-reconcile-report.txt`.

---

## Prompt para replicar el análisis

Este prompt reproduce el trabajo completo (análisis de flujos, informe con marca Illumio y CSV en la nomenclatura del kit) a partir de un export de Explorer/Traffic. Pegalo en Claude con el export adjunto y, si las tenés, capturas de pantalla de las etiquetas existentes en el PCE.

````text
Sos un ingeniero de preventa de Illumio. Te adjunto un export de tráfico del PCE (Explorer / vista Traffic) de una prueba de concepto de microsegmentación y necesito que reproduzcas un análisis completo y sus entregables. Trabajá en español, con lenguaje técnico y directo, sin adjetivos de relleno.

## Insumos
- Export de Explorer/Traffic en CSV o XLSX (adjunto). Puede venir de la consola del PCE o de `workloader traffic`; normalizá los nombres de columna a un esquema común: src_ip, src_name, src_hostname, src_enforcement, src_labels (una por tipo), src_fqdn, dst_* equivalentes, port, proto, process, username, num_flows, bytes_in, bytes_out, connection_state, policy_decision, reported_by, first_detected, last_detected.
- Opcional: capturas de pantalla de la lista de etiquetas y tipos de etiqueta del PCE, y la lista de workloads con VEN. Si las adjunto, usá exactamente esos nombres de etiqueta; si no, proponé valores nuevos con la nomenclatura de abajo.
- Contexto que te doy en el mensaje: alcance de la POC (qué hosts tienen VEN, qué grupos), nombre corto del grupo para el prefijo de prioridad (por ejemplo G1, G2, C3) y cualquier dato de red conocido (VLAN de usuarios, rangos de nube, sitios).

## Normalización (hacela antes de analizar y documentá qué encontraste)
1. IPs mutiladas por Excel: celdas numéricas, fechas o notación científica donde debería haber una IP (por ejemplo 10.0.4.10 convertido en 10.004 o en una fecha). Reconstruilas cuando sea inequívoco y marcá las que no; nunca inventes octetos.
2. Fechas en formatos mezclados (dd/mm/yyyy, mm/dd/yyyy, ISO, con y sin hora): unificá a ISO 8601 y explicá el criterio de desambiguación.
3. Truncamiento: contá las filas. Si el archivo tiene exactamente el tope de la consulta (5.000, 100.000, 200.000 o cualquier número redondo que coincida con un límite) o si un solo par origen-destino ocupa la mayoría de las filas (un escaneo de puertos, por ejemplo), declaralo como export truncado, cuantificá cuánto ocupa el ruido y recomendá repetir el export con filtros y ventanas más cortas. No afirmes que el tráfico de un host está completo si el export está truncado.
4. Deduplicá filas idénticas, separá tráfico unicast de broadcast/multicast y descartá lo que no sea IP.

## Criterio de clasificación de cada peer sin VEN
- WORKLOAD NO GESTIONADO (UMWL): servidor concreto con rol identificable a partir de la evidencia: resolutores DNS, NTP internos, servidores y proxies de monitoreo (Zabbix, Nagios, SolarWinds, OEM), colectores de logs (Splunk, syslog), backup (NetBackup, Veeam, Commvault), bases de datos (Oracle, SQL Server, MySQL, PostgreSQL), balanceadores y front-ends de una aplicación, nodos de clúster, servidores de aplicación, bastiones u orígenes SSH/RDP administrativos, escáneres de vulnerabilidades, relays SMTP, colas de mensajes, controladores de dominio, gestores de agentes (EDR, antivirus). Uno por IP.
- LISTA DE IP: rangos amplios o peers que no son servidores: VLAN de usuarios y endpoints, subredes completas de una aplicación, metadata de nube (169.254.169.254, que en OCI es además el resolver de la VCN), NTP público, consolas SaaS (EDR, monitoreo), rangos publicados por un proveedor de nube, Internet.
- Cuando un grupo de IPs comparte rol y aplicación (front-ends, nodos RAC), proponé UMWL por IP con las mismas etiquetas y, además, una lista de IP de conveniencia para escribir la regla hoy y migrar a etiquetas después.

## Evidencia (obligatoria en cada inferencia)
Cada rol asignado a una IP sin VEN es una inferencia. Para cada UMWL y cada lista citá: puertos y protocolo, dirección del flujo (quién consume a quién), proceso y usuario del lado del VEN, cantidad de flujos y bytes, patrón temporal (continuo, horario, puntual) y nivel de confianza Alta / Media / Baja con una frase de razonamiento. Si el rol no se puede sostener con evidencia, ponelo como "(a confirmar)" y asignale prioridad 3.

## Modelo de etiquetas
Cuatro tipos por defecto (Role, Application, Environment, Location) con prefijo en el valor:
- R_<Rol>   (R_DNS, R_NTP, R_Monitoring, R_LogCollector, R_Backup, R_Database, R_LoadBalancer, R_AppServer, R_Bastion, R_Scanner, R_Mail, R_Messaging, R_DomainController, R_AgentMgmt)
- A_<App>   (una por aplicación de negocio, más A_CoreInfra, A_Observability, A_SecurityTools, A_AdminAccess)
- E_<Env>   (E_Prod, E_QA, E_Dev; si no hay evidencia, E_Prod "a confirmar")
- L_<Loc>   (sitio o nube: L_DC1, L_OCI, L_AWS, L_Azure, L_GCP)
Si el PCE ya tiene una convención (capturas adjuntas), respetala. Explicá que la etiqueta de Application es la que sostiene el ringfence y que Illumio aplica OR dentro de un tipo y AND entre tipos.

## Hallazgos de ciberseguridad
Listá hallazgos con ID (S-01…), severidad (Alta/Media/Baja/Info), evidencia concreta del export y recomendación. Buscá al menos: escaneos sin ventana declarada, software fuera de soporte (deducido de procesos/usuarios/banners), protocolos en claro (FTP, Telnet, HTTP con credenciales, SMB1), bases de datos alcanzadas por direccionamiento público, tormentas de DNS u otros volúmenes anómalos, NTP/DNS públicos directos desde servidores, SSH/RDP directo como root/Administrator, puertos de gestión expuestos (AJP, JMX, RMI, WinRM), broadcast/multicast no explicado, inventario de agentes con salida propia. Cada hallazgo debe poder verificarse en Explorer con un filtro de IP y puerto.

## Verificación de hechos
Antes de afirmar un puerto, un proceso, una versión, una fecha de fin de soporte o un comportamiento de Illumio, verificalo contra documentación oficial vigente (product-docs-repo.illumio.com para Illumio; la documentación del fabricante para Zabbix, Oracle, Splunk, NetBackup, Trend Micro, OCI/AWS/Azure) y citá la URL. Lo que no puedas verificar, marcalo como supuesto. No inventes hostnames, nombres de personas ni datos comerciales.

## Entregables
1. Informe en español, orientado al cliente pero SIN el nombre del cliente ni de personas (usá "el cliente", "la POC"), en HTML y PDF con la skill illumio-branded-reports. Secciones: resumen ejecutivo con indicadores (filas, ventana, workloads con VEN, IPs sin gestionar, porcentaje de ruido), datos y método (incluida la normalización y el truncamiento), inventario de workloads con VEN y etiquetas propuestas, workloads no gestionados a crear (tabla: nombre/FQDN, IP, rol inferido y evidencia, confianza, etiquetas R/A/E/L, prioridad), redes internas y listas de IP (tabla: nombre, miembros, razonamiento, uso en reglas), modelo de etiquetas, patrones de acceso observados con un diagrama en capas, hallazgos de ciberseguridad, política inicial propuesta (servicios y reglas de ringfence por aplicación más bloque común de gestión), próximos pasos, anexos y fuentes por sección. Diagramas como SVG inline con la paleta de la skill, sin texto rotado ni fuera de las cajas.
2. Workbook XLSX de objetos a cargar con hojas: UMWL, IP lists, Labels, Services, Rules (borrador) y Findings.
3. CSV de workloads no gestionados en el formato de workloader wkld-import, UNA FILA POR IP, columnas exactas:
   hostname,name,interfaces,description,role,app,env,loc,review
   - hostname: vacío siempre (los FQDN del export pueden estar anonimizados).
   - name: "<Rol descriptivo> <IP>", por ejemplo "Zabbix Server 10.43.43.21". Debe ser único.
   - interfaces: "eth0:<ip>".
   - description: "[<grupo> P<prioridad> conf:<Alta|Media|Baja>] <comentario con la evidencia>", por ejemplo "[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP e ICMP en atun4 y wlsfp1a (≈600.000 flujos) y recibe 10051".
   - role, app, env, loc: valores con prefijo R_/A_/E_/L_. Celda vacía si no hay etiqueta.
   - review: PENDING.
   Prioridad 1 = necesario para el ringfence (front-ends, bases de datos, plano de gestión diario); 2 = útil pero confirmable después; 3 = identificar antes de crear.
4. CSV de listas de IP en el formato de workloader ipl-import, columnas exactas:
   name,description,include,exclude,fqdns
   - name en minúsculas con prefijo ipl- (ipl-dns-corp, ipl-oci-metadata); include con IPs, CIDR o rangos separados por ";"; fqdns para nombres.
5. Un bloque final "Supuestos y verificaciones pendientes" con todo lo que el cliente debe confirmar antes de cargar.

Entregá primero un resumen de lo que encontraste en la normalización y una propuesta de agrupación; después el informe y los CSV.
````

### Variante corta: actualizar con un export nuevo (v2)

````text
Te adjunto un export nuevo de Explorer/Traffic del mismo alcance que el análisis anterior (adjunto también el informe previo y los CSV umwl-import e ipl-import ya cargados o propuestos). Necesito la versión v2:

1. Normalizá el export nuevo con las mismas reglas (IPs mutiladas, fechas, truncamiento, deduplicación) y compará ventana, filas y ruido con el export anterior.
2. Diferencias contra los objetos ya propuestos: peers nuevos (proponer UMWL o lista con evidencia, confianza y prioridad), peers que desaparecieron (marcar como "sin tráfico en la ventana nueva", no borrar), cambios de rol o de etiqueta sugeridos por la evidencia nueva, hallazgos nuevos o cerrados.
3. Entregá:
   - <grupo>-umwl-import-v2.csv completo (no solo el delta), mismas columnas y nomenclatura (hostname vacío, name "<Rol> <IP>", interfaces "eth0:<ip>", description "[<grupo> P<n> conf:X] …", etiquetas R_/A_/E_/L_, review = PENDING para filas nuevas y UPDATED para filas cuyo comentario o etiquetas cambiaron, UNCHANGED para el resto).
   - <grupo>-ipl-import-v2.csv completo.
   - Una tabla de cambios (IP, antes, después, motivo) y una sección de hallazgos actualizada.
   - Si hace falta el informe completo, generalo con la skill illumio-branded-reports; si no, un addendum de dos páginas.
Verificá contra documentación oficial cualquier puerto, versión o comportamiento nuevo que afirmes y citá la URL. Sin nombre del cliente ni de personas.
````

Después de obtener el CSV v2, la TUI hace el resto: en el paso 4 las filas cuya IP ya existe salen como EXISTS-UNMANAGED y se actualizan por `href` (etiquetas y descripción); las nuevas se crean.

---

## Solución de problemas

| Síntoma | Causa y solución |
|---|---|
| macOS no deja ejecutar `workloader` ("no se puede abrir porque no se pudo verificar el desarrollador") | Cuarentena de Gatekeeper al descargar el zip. `xattr -d com.apple.quarantine ./workloader && chmod +x ./workloader`. La TUI lo hace al descargar. |
| `workloader` en Apple Silicon | El release de macOS es un binario Intel y corre con Rosetta 2 (`softwareupdate --install-rosetta` si no está instalado). Para un binario nativo: `brew install go`, `git clone https://github.com/brian1917/workloader.git && cd workloader && go build -o ../workloader .` (la TUI ofrece esta opción). |
| `pce-add` pide correo y contraseña aunque pasaste `--api-user` | Falta el flag `--api-key`; sin él workloader ignora `--api-user/--api-secret/--org`. El kit ya lo incluye. |
| 401/403 al importar; el dry run funciona | La API key hereda el rol del usuario. Creala con un usuario Global Organization Owner o Global Administrator (un Workload Manager con alcance puede crear workloads, pero no etiquetas nuevas ni listas de IP), o usá una service account con esos permisos. Verificá también el `org id`. |
| Error TLS con PCE on-prem y certificado interno | `pce-add … --disable-tls-verification true` solo en laboratorio; en producción instalá la CA en el llavero. |
| El PCE que muestra `pce-list` no es el del cliente (u otra organización) | Estás en la carpeta equivocada (el directorio actual decide qué `pce.yaml` se usa) o `ILLUMIO_CONFIG` apunta a otro archivo. Una carpeta por cuenta + PCE; `unset ILLUMIO_CONFIG`; `--pce <nombre>` solo si el `pce.yaml` tiene varios perfiles. |
| El dry run dice que va a actualizar un workload que no esperabas | Match por `hostname` o `name` con un objeto existente. Revisá `runs/<ts>/dry-*.log`; renombrá la fila (paso 4/5) o dejá que la reconciliación por IP lo resuelva. |
| Names duplicados / "se cargó uno solo" | workloader trata dos filas con el mismo `hostname` (o el mismo `name` sin hostname) como el mismo workload. La TUI agrega la IP como sufijo; a mano, hacé único el `name`. |
| Una columna de etiqueta no se aplicó | El tipo de etiqueta no existe en el PCE; `wkld-import` ignora la columna. Crealo en la consola (Settings → Label Settings) o con `./workloader label-dimension-import tipos.csv --update-pce` (CSV con `key,display_name`), y volvé a correr. |
| El export de Explorer tiene exactamente N filas redondas | Está truncado. Subí el límite de resultados, filtrá por aplicación, excluí escáneres, dividí la ventana de tiempo y volvé a exportar (ver sección de extracción). |
| `wkld-import` crea la etiqueta con otra capitalización | Los valores son sensibles a mayúsculas salvo `--ignore-case`. Usá exactamente los valores que muestra `label-export`. |
| Lote fallido en el paso 7 | Revisá `runs/<ts>/create-chunkNNN.log`; el CSV de ese lote queda en `create-chunkNNN.csv` para reintentarlo a mano con `wkld-import … --umwl --update-pce`. |

---

## Guía explicativa (PDF)

La guía explica con diagramas el flujo completo (export → análisis → CSV → kit → workloader → PCE), los requisitos y la regla de una carpeta por cuenta + PCE, la inicialización de la carpeta de trabajo, la secuencia de pasos de la TUI con sus puntos de decisión, la relación entre el kit y los subcomandos de workloader, el mapeo de columnas del CSV a campos y etiquetas del PCE y la solución de problemas. Está en dos idiomas, con el mismo contenido:

- Español: [docs/Guia-workloader-import-kit.pdf](docs/Guia-workloader-import-kit.pdf) (HTML autocontenido: `docs/Guia-workloader-import-kit.html`).
- English: [docs/Guide-workloader-import-kit-EN.pdf](docs/Guide-workloader-import-kit-EN.pdf) (self-contained HTML: `docs/Guide-workloader-import-kit-EN.html`).

## Fuentes consultadas

- workloader (brian1917): README, releases v12.1.9, `cmd/root.go`, `cmd/wkldimport/cmd.go`, `cmd/iplimport/iplimport.go`, `cmd/wkldexport/headers.go`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension/import.go`, `cmd/traffic/traffic.go` — https://github.com/brian1917/workloader
- Illumio Core 25.4 — Visualization — About the Visualization Tools (Explore, Traffic, límites, consultas asíncronas) — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.4/Visualization/out/en/visualization-tools/about-the-visualization-tools.html
- Illumio Core 25.1 — Visualization — Traffic Table (filtros, Export) — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.1/Visualization/out/en/visualization-tools/traffic-table.html
- Illumio Core 22.5 — REST API — Explorer (async_queries, max_results 200.000) — https://product-docs-repo.illumio.com/Tech-Docs/Core/22.5/REST-APIs/out/en/core-22-5-rest-api-developer-guide/visualization/explorer.html
- Illumio Core 24.2 — REST API — API Keys (My API Keys, permisos) — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/REST-APIs/out/en/rest-apis/authentication-and-api-user-permissions/api-keys.html
- Illumio Core 24.5 — Security Policy — Workload Setup Using PCE Web Console (Add Unmanaged Workload) — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.5/Security-Policy/out/en/security-policy-guide-24-5/workloads/workload-setup-using-pce-web-console.html
- Illumio Core 24.2 — Getting Started — Policy Objects — https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/Getting%20Started/out/en/policy-overview/policy-objects.html
- Illumio Core 25.4 — Security Policy — Create a Label Type — https://product-docs-repo.illumio.com/Tech-Docs/Core/25.4/Security-Policy/out/en/security-policy-objects/about-labels-and-label-groups/label-types/create-a-label-type.html

## Licencia

MIT. Ver [LICENSE](LICENSE).
