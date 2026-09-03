English: [README.md](README.md)

# illumio-workloader-import-kit

`umwl-tui` es una aplicación de terminal de pantalla completa (Go, un solo binario estático) que carga en un PCE de Illumio los **workloads no gestionados (UMWL)** y las **listas de IP** que propone un análisis de flujos, usando [workloader](https://github.com/brian1917/workloader) como motor de importación: reconcilia cada fila por IP contra el inventario real, hace dry run y escribe por lotes solo después de una confirmación explícita. Alrededor de ella, el repositorio documenta el método completo para ir de un export de Explorer/Traffic a los objetos cargados: qué exportar del PCE, cómo pseudonimizarlo, las instrucciones del Proyecto de Claude que producen el informe y los CSV, la convención de nombres y el contrato CSV de workloader. Los loaders anteriores en Python de terminal plana viven en `legacy/` como alternativa.

> Verificado contra **workloader v12.1.9** (release del 12 de junio de 2025; código en `master` consultado el 3 de septiembre de 2026) e **Illumio Core 24.x/25.x** (documentación en `product-docs-repo.illumio.com`). Los nombres de comandos, flags y cabeceras CSV de este README se tomaron del código fuente de workloader (`cmd/root.go`, `cmd/wkldimport`, `cmd/iplimport`, `cmd/wkldexport`, `cmd/pcemgmt/addpce.go`, `cmd/labeldimension`, `cmd/traffic`). Si usás otra versión, confirmá con `./workloader <comando> --help`.

## Inicio rápido

1. Carpeta de trabajo (una por cuenta de Illumio + PCE, sección siguiente; todos los comandos de abajo corren ahí): `mkdir -p ~/illumio/cliente-a/scp57-org12 && cd ~/illumio/cliente-a/scp57-org12`
2. El kit (repositorio privado: antes `gh auth login` o una clave SSH cargada en GitHub): `git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit`
3. El binario: `gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS && shasum -a 256 -c SHA256SUMS --ignore-missing && mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui` (o compilalo: `(cd kit && make build) && cp kit/umwl-tui .`).
4. workloader + `pce.yaml` + prueba de conexión: `./umwl-tui --setup-only` (descarga workloader, ejecuta `pce-add --api-key`, prueba con `label-dimension-export`).
5. Exportar desde el PCE (consola: guía §4 "Exportar desde el PCE"; o workloader): `./workloader traffic --start 2026-08-17 --end 2026-09-01 --max-results 200000 --output-file TrafficData.csv`, y después `./workloader wkld-export --output-file pce-workloads.csv`, `./workloader label-export --output-file pce-labels.csv`, `./workloader label-dimension-export --output-file pce-label-types.csv`.
6. Pseudonimizar antes de compartir: `python3 kit/anonymize_export.py anon TrafficData.csv -o TrafficData.anon.csv --map anon-map.json --customer "Cliente A" --domain cliente-a.com`
7. Análisis en un Proyecto de Claude: instrucciones = `kit/docs/prompts/context.md`, mensaje = `kit/docs/prompts/prompt-short.md`, adjuntos = `TrafficData.anon.csv` + los exports `pce-*.csv`. Devuelve el informe, `<grupo>-umwl-import.csv` y `<grupo>-ipl-import.csv`.
8. Nombres reales de vuelta en la propuesta: `python3 kit/anonymize_export.py deanon C4-umwl-import.csv -o cliente-a-umwl-import.csv --map anon-map.json` (lo mismo con el CSV de listas de IP si trae FQDN).
9. Cargar: `./umwl-tui cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1` — diez pasos, dry run, confirmación "Write to the PCE?", lotes, verificación.
10. El reporte de la corrida: `runs/<timestamp>/report.md` (y `report.json`, los CSV por categoría y todos los logs de workloader).

## Importante: una carpeta de trabajo por cuenta (organización) de Illumio + PCE

workloader resuelve la conexión al PCE leyendo, en este orden, `--config-file`, la variable `ILLUMIO_CONFIG` y, si no hay ninguna, **`./pce.yaml` en la carpeta de trabajo**. `umwl-tui`, además, escribe en la carpeta de trabajo `runs/<timestamp>/` con el inventario exportado del PCE, los CSV finales y los logs de cada lote, y `anonymize_export.py` guarda ahí `anon-map.json`.

La unidad de aislamiento no es "el cliente" ni "el PCE" sino cada combinación **cuenta u organización de Illumio + PCE**, es decir, cada perfil distinto de `pce.yaml` (FQDN, puerto, org id y API key):

- Una cuenta puede tener varios PCE (SaaS y on-prem, producción y DR, regiones): cada uno es una carpeta.
- Un PCE SaaS (un FQDN) aloja varias organizaciones; cada `org id` es un tenant distinto y lleva su propia carpeta, aunque el FQDN sea el mismo. El `org id` es el número que aparece en la URL de la consola (`…/orgs/<id>/…`).
- Otra cuenta: misma estructura, nunca el mismo `pce.yaml`.

Si en una misma carpeta conviven `pce.yaml` de dos tenants (o un `pce.yaml` con varios PCE y te olvidás del `--pce`), el riesgo es concreto: **importar los objetos de un cliente en el PCE o la organización de otro**. La regla es simple: **una carpeta de trabajo por cuenta + PCE**, con su propio `pce.yaml`, sus CSV y sus `runs/`. Nomenclatura sugerida: `~/illumio/<cuenta>/<pce-o-org>/`.

```
~/illumio/
├── cliente-a/
│   ├── scp57-org12/                  # cuenta cliente-a, PCE SaaS scp57, org 12
│   │   ├── umwl-tui                  # la aplicación (binario del release o make build)
│   │   ├── workloader                # binario; lo descarga umwl-tui (o symlink a uno compartido)
│   │   ├── pce.yaml                  # SOLO esta cuenta + PCE; lo crea pce-add; nunca copiarlo
│   │   ├── anon-map.json             # mapa de pseudónimos; nunca sale de esta carpeta
│   │   ├── cliente-a-umwl-import.csv
│   │   ├── cliente-a-ipl-import.csv
│   │   ├── runs/20260903-101500/     # una subcarpeta por corrida
│   │   └── kit/                      # clon de este repositorio (docs, prompts, anonimizador, ejemplos)
│   └── onprem-prod/                  # misma cuenta, otro PCE: otra carpeta, otro pce.yaml
└── cliente-b/<pce-o-org>/            # otra cuenta, misma estructura
```

Antes de cada corrida, `umwl-tui` ejecuta `workloader pce-list` y te muestra el PCE que va a usar; leé ese nombre y el FQDN antes de aceptar. Si el `pce.yaml` tiene más de un PCE, pasá siempre `--pce <nombre>`. workloader admite varios perfiles en un solo `pce.yaml` (con `default_pce_name` y el flag `--pce`), pero el kit no se apoya en eso a propósito: un `--pce` olvidado o un default equivocado cargaría los objetos en el tenant incorrecto.

## Contenido del repositorio

| Ruta | Qué es |
|---|---|
| `cmd/umwl-tui/`, `internal/`, `go.mod`, `Makefile`, `testdata/` | **`umwl-tui`** (Go, Bubble Tea): la aplicación de pantalla completa. `internal/engine` es la lógica de workloader/CSV sin E/S de terminal; `internal/tui` el modelo, los pasos, los modales y el selector de archivos; `testdata/` el mock de workloader y los CSV que usan las pruebas sin terminal. `make build`, `make test`, `make dist`. |
| `anonymize_export.py` | Pseudonimización consistente y reversible de los exports (`anon`) y restauración de los nombres reales en los CSV propuestos (`deanon`). Python 3, biblioteca estándar. |
| `docs/Guide-umwl-tui-EN.{pdf,html}`, `docs/Guia-umwl-tui-ES.{pdf,html}` | La guía unificada (30 páginas, inglés y español): flujo de trabajo, requisitos, inicialización, exportar desde el PCE con pantallas esquemáticas, anonimización, el análisis con Claude, umwl-tui paso a paso, ciclo v2, solución de problemas. |
| `docs/prompts/context.md`, `docs/prompts/prompt-short.md` | Instrucciones del Proyecto de Claude (el método completo) y el mensaje por conversación (v1 y v2). |
| `docs/export-columns.txt`, `docs/img/*.svg` | Cabecera de un export de Traffic 25.x; los esquemas que usa la guía. |
| `docs/SPEC-umwl-tui.md`, `CLAUDE.md` | El contrato de `umwl-tui` (máquina de estados, UI, contrato con workloader, requisitos de seguridad, backlog) y los comandos de build/test/release, convenciones y lista de control de seguridad para sesiones de Claude Code. |
| `examples/*-umwl-import.csv`, `examples/*-ipl-import.csv` | CSV propuestos de una prueba de concepto (IPs de laboratorio del cliente: plantillas de formato, no para cargar tal cual). `cliente3-*-v2.csv` son el **formato recomendado**: etiquetas `R_/A_/E_/L_`, `name` = rol + IP, `hostname` vacío, `interfaces` = `eth0:<ip>`, listas `IPL_<Sitio>_<Uso>`. |
| `legacy/` | `umwl_loader.py` y `reconcile_umwl.py`, los predecesores de terminal plana (ver "Legacy" más abajo y `legacy/README.md`). |
| `.gitignore` | Excluye `workloader`, `umwl-tui`, `pce.yaml`, `runs/`, `*.log`, `anon-map.json`, `*.anon.csv`, `*.real.csv`, `dist/`. **Nunca subas `pce.yaml`: contiene la API key.** |

## Requisitos

- **macOS** (probado en Apple Silicon; el flujo es el mismo en Intel y en Linux). `umwl-tui` se publica como binario estático para darwin/arm64, darwin/amd64, linux/amd64 y linux/arm64; compilarlo requiere **Go 1.24+**. **Python 3.9+** (solo biblioteca estándar) para `anonymize_export.py` y los loaders legacy.
- **workloader v12.x**. No requiere instalación: es un binario. El release de macOS se publica como `mac-<versión>.zip`; en Apple Silicon corre vía Rosetta 2. `umwl-tui --setup-only` lo descarga, o lo compila desde el fuente si Go está instalado.
- Una **API key del PCE con permisos de escritura**. La key hereda el rol del usuario que la crea: si el usuario es de solo lectura, `wkld-import --update-pce` falla. Para crearla: menú de usuario (arriba a la derecha) → **My API Keys** → **Add**; guardá el **Authentication Username** (`api_…`) y el **Secret**, que solo se muestran una vez. También sirve una service account con permisos equivalentes. **Acceso de red al PCE por HTTPS** (puerto 443 en SaaS, 8443 típico on-prem). En SaaS el `org id` es el número que aparece en la URL de la consola (`…/orgs/<id>/…`).

## Inicialización de la carpeta de trabajo

Se hace una vez por combinación cuenta + PCE; después, cada carga es un solo comando. Los pasos 1–4 del inicio rápido son la versión corta.

1. **Carpeta**: `mkdir -p ~/illumio/cliente-a/scp57-org12 && cd ~/illumio/cliente-a/scp57-org12`. Todo lo que sigue corre con esa carpeta como directorio actual: `./umwl-tui`, `./workloader`, `./pce.yaml` y `./runs` quedan junto a los CSV, no dentro del clon.
2. **Kit**: `git clone https://github.com/roschereric/illumio-workloader-import-kit.git kit` (equivalente: `gh repo clone …`; sin git: Code → Download ZIP, descomprimido como `kit/`). Los CSV viven en la carpeta de trabajo, no en `kit/`; `kit/examples/` contiene solo plantillas. Actualizar con `git -C kit pull`. Para compartir un solo clon entre carpetas: `ln -s ~/illumio/kit-shared kit`. Después, `umwl-tui` mismo: binario del release o `make build` (ver "Instalación" más abajo).
3. **workloader y pce.yaml**: `./umwl-tui --setup-only`. Busca `./workloader` (después el PATH, o la ruta de `--workloader`). Si no lo encuentra ofrece descargar el release (`mac-<versión>.zip`, con `curl` + `unzip` y `xattr -d com.apple.quarantine`) o clonar el repositorio de workloader en `workloader-src/` y compilar con `go build`. Después corre `pce-list`; si no hay `pce.yaml`, o workloader responde "no pce configured", va directo a `pce-add`: pide nombre corto, FQDN, puerto, API user, API secret y org id, ejecuta `pce-add --api-key …` con esos valores (así workloader nunca pide correo y contraseña), vuelve a mostrar `pce-list` y prueba la conexión con un `label-dimension-export`. (Alternativa sin el binario: `python3 kit/legacy/umwl_loader.py --setup-only` hace el mismo paso.)
4. **Prueba de cordura (solo lectura)**: `./workloader pce-list && ./workloader wkld-export --output-file sanity.csv` y mirá los hostnames: ¿es el inventario del tenant esperado? Un export de solo lectura demuestra que la API key funciona antes de escribir nada.
5. **Otra cuenta u otro PCE**: repetir en una carpeta nueva. Nunca copiar `pce.yaml`. Varios PCE en un mismo `pce.yaml` no es el esquema del kit; si igual existe, pasar siempre `--pce <nombre>`.

## umwl-tui

El mismo flujo de diez pasos que el kit tuvo siempre, como una aplicación de terminal real (estilo htop / Midnight Commander): barra de estado con el PCE, la carpeta de la corrida y el contador de pasos; lista de pasos con su estado a la izquierda; panel del paso activo a la derecha (tablas que se recorren con las flechas, detalle "propuesto vs. en el PCE" lado a lado, barras de progreso); salida de workloader en vivo abajo; y una barra de teclas que cambia por paso. Cada decisión que importa (conflictos, escrituras al PCE, lotes fallidos) es un modal; no se escribe nada antes de la confirmación "Write to the PCE?" posterior al dry run. La interfaz está en inglés.

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

Los binarios precompilados van adjuntos a los releases de GitHub (`umwl-tui-darwin-arm64`, `umwl-tui-darwin-amd64`, `umwl-tui-linux-amd64`, `umwl-tui-linux-arm64`, más `SHA256SUMS`). En una Mac, en la carpeta de trabajo:

```bash
gh release download --repo roschereric/illumio-workloader-import-kit --pattern 'umwl-tui-darwin-arm64' --pattern SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
mv umwl-tui-darwin-arm64 umwl-tui && chmod +x umwl-tui && xattr -d com.apple.quarantine umwl-tui
```

O compilarlo (Go 1.24+): `(cd kit && make build) && cp kit/umwl-tui .`. `make dist` compila todas las plataformas en `dist/` con checksums; `make test` corre las pruebas sin terminal contra `testdata/mock-workloader.py`.

### Uso

```bash
./umwl-tui --setup-only                                   # solo el paso 0: workloader, pce.yaml (pce-add --api-key), prueba de conexión
./umwl-tui cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1
```

| Flag | Significado |
|---|---|
| `<csv>` | CSV de workloads propuestos, una fila por IP (formato `wkld-import`). Opcional: sin él se abre el selector de archivos. |
| `--setup-only` | Solo instalar/verificar workloader y la conexión al PCE; no carga nada. |
| `--ipl <csv>` | CSV de listas de IP (formato `ipl-import`) para el paso 9. |
| `--pce <nombre>` | Nombre del PCE en `pce.yaml` (se pasa como `--pce` a workloader). Obligatorio si `pce.yaml` tiene más de un PCE. |
| `--priority 1` o `1,2` | Cargar solo las filas cuyo `description` empiece con `[.. P<n> ..]` para esas prioridades. |
| `--workloader <ruta>` | Ruta al binario (por defecto `./workloader`, después el PATH). |
| `--chunk 20` | Filas por llamada a `wkld-import`: granularidad del progreso y radio de impacto de un error. |
| `--runs ./runs` | Carpeta donde se crea `runs/<timestamp>/`. |

### Los diez pasos

| Paso | Qué hace | Toca el PCE |
|---|---|---|
| 0 Preflight | Localiza workloader (o lo descarga/compila), muestra el PCE (`pce-list`), lo configura con `pce-add --api-key` si hace falta, prueba la conexión, crea `runs/<ts>/`. | Solo lectura |
| 1 Load CSV | Columnas obligatorias (`hostname`, `name`, `interfaces`), IPs válidas, IPs repetidas, filtro `--priority`, hostnames o names duplicados (les agrega la IP como sufijo), resumen de valores por etiqueta. | No |
| 2 PCE inventory | `wkld-export`, `label-export`, `label-dimension-export` a `runs/<ts>/`. Indexa cada workload del PCE por sus IPs (`interfaces` y `public_ip`) y marca los gestionados (`managed` o `ven_href`). | Solo lectura |
| 3 Labels | Columnas del CSV que no son un tipo de etiqueta del PCE → descartar o salir a crear el tipo. Lista los valores nuevos que se crearán y pide confirmación. | No |
| 4 Reconcile by IP | Cada fila se clasifica: **NEW** (la IP no existe), **EXISTS-UNMANAGED** (la IP pertenece a un UMWL: por defecto se actualizan etiquetas/descripción del existente, por `href`), **CONFLICT-MANAGED** (la IP pertenece a un workload con VEN: por defecto se omite), **CONFLICT-MULTIPLE** (varios workloads comparten la IP). Por fila: actualizar, omitir, crear igual, renombrar, o aplicar la decisión a todos los del mismo tipo. | No |
| 5 Review new | Tabla de los nuevos; aceptar todos o revisar uno por uno editando name/hostname/description/etiquetas. Escribe `to-create.csv`, `to-update.csv`, `skipped.csv`. | No |
| 6 Dry run | `wkld-import to-create.csv --umwl --update=false --match name` y `wkld-import to-update.csv --match href`, sin `--update-pce`; muestra las líneas relevantes del log de workloader. Un dry run que reporta `cannot be blank` o `nothing to be done` se trata como fallido. Pide confirmación explícita antes de seguir. | No |
| 7 Execute | Divide en lotes de `--chunk` filas y corre los mismos comandos con `--update-pce --no-prompt` por lote, con barra de progreso; ante error: reintentar, saltar el lote o abortar. | **Sí** |
| 8 Verify | Nuevo `wkld-export` y confirma que cada IP creada aparece como workload. | Solo lectura |
| 9 IP lists | Con `--ipl`: dry run de `ipl-import`, confirmación, `ipl-import … --update-pce --no-prompt`. | **Sí** |
| 10 Report | `runs/<ts>/report.md` y `report.json`: creados, actualizados, omitidos, lotes fallidos, etiquetas creadas, verificación y todos los comandos ejecutados con su código de salida. | No |

### Teclas, selector de archivos, diálogos, carpeta de la corrida

- **Globales**: `tab` pasa el foco al panel de log (se recorre con las flechas), `?` ayuda, `q` salir (pregunta una vez que empezó el trabajo), `ctrl+c` salir. Tablas: `↑↓ j k pgup pgdown g G`. **Reconciliación (paso 4)**: `u` actualizar el objeto existente, `s` omitir, `c` crear igual, `r` renombrar y actualizar, `U`/`S` aplicar a todas las filas del mismo tipo, `n` siguiente fila sin decidir. **Revisión (paso 5)**: `e` editar name/hostname/description/etiquetas, `s` omitir. **Diálogos**: cada escritura (paso 7 y paso 9) está detrás de un modal rojo "Write to the PCE?"; un lote fallido abre reintentar / saltar / abortar; `esc` cancela un formulario; los modales capturan todas las teclas mientras están abiertos.
- **Selector de archivos**: se abre después del preflight cuando no se pasó un CSV (ruta editable arriba, carpetas y CSV abajo, `..` para subir, `tab` o `/` para escribir una ruta exacta, `~` se expande), luego pide el CSV opcional de listas de IP y el filtro de prioridad. Un CSV que no pasa la validación muestra el motivo y vuelve a abrir el selector.
- **Carpeta de la corrida** `runs/<timestamp>/`: inventarios del PCE antes y después, `to-create.csv`, `to-update.csv`, `skipped.csv`, un CSV y un log de workloader por lote (`create-chunkNNN.*`, `update-chunkNNN.*`), los logs del dry run, `report.md` y `report.json` (con cada comando de workloader y su código de salida, el nombre del PCE y la carpeta de trabajo). El API secret nunca llega a los logs ni al reporte. Notas de diseño y desarrollo: `docs/SPEC-umwl-tui.md` y `CLAUDE.md`.

## Exportar los flujos desde el PCE

El insumo del análisis es el CSV de tráfico del PCE. En las consolas 22.x–23.x la vista se llama **Explorer**; en las consolas 24.x/25.x los mismos datos están en la vista **Traffic** (tabla) dentro de **Explore** en el menú izquierdo, junto a **Map**. La guía (§4) muestra las pantallas de forma esquemática; la mecánica:

1. **Ventana de tiempo**: al menos 7 días que incluyan un fin de semana y, si existen, ventanas de respaldo y de escaneo.
2. **Filtros**: en *Source* (consumer) y *Destination* (provider) incluí de un lado los workloads con VEN (o su etiqueta de aplicación) y dejá el otro lado abierto, con "or" entre ambos lados para capturar entrada y salida; en *Service*, puertos, protocolos y procesos.
3. **Límite de resultados**: la consola muestra hasta 10.000 conexiones por página y 100.000 en pantalla; el CSV descargado puede llegar a 200.000 en un PCE standalone. **Un export exactamente en el tope es un export truncado** (el que analizamos venía con exactamente 5.000 filas, el 93 % de ellas un solo escaneo de puertos). Subí el límite al máximo y, si aun así se llena, dividí por aplicación/grupo de workloads y ventanas más cortas, y excluí el origen del escáner.
4. **Run** (las consultas asíncronas aparecen después en **Load Results**, arriba a la derecha), y luego **Export** → CSV, con un nombre que incluya cliente, alcance y fechas (`cliente-a_grupo1_2026-08-17_2026-09-01.csv`). **Abrí el CSV como texto** (editor, `head`, Python). Si lo abrís en Excel, importalo con el asistente marcando las columnas de IP como **Texto**: Excel convierte `10.0.4.10` en número o fecha. Lo mismo con las columnas de fecha.

Alternativas al export manual:

- **workloader** (`workloader traffic`, "Export traffic data"): `--start` / `--end` (`yyyy-mm-dd` o `yyyy-mm-ddTHH:mm:ss`, GMT; por defecto 88 días atrás y mañana), `--max-results` (por defecto 100.000, máximo 200.000), `--incl-src-file` / `--excl-src-file` / `--incl-dst-file` / `--excl-dst-file` (archivos con hrefs de `label-export`, `ipl-export`, `wkld-export`), `--incl-svc-file` / `--excl-svc-file`, `--excl-allowed` / `--excl-potentially-blocked` / `--excl-blocked`, `--output-file`. El comando `explorer` sigue registrado como `legacy-explorer`; usá `traffic`. Sus columnas no son idénticas a las de la consola; las instrucciones del análisis normalizan ambas.
- **API REST asíncrona** (la que usa la consola): `POST /api/v2/orgs/<org>/traffic_flows/async_queries` con `query_name`, `sources`, `destinations`, `services`, `policy_decisions`, `start_date`, `end_date`, `max_results` (límite 200.000); después `GET …/traffic_flows/async_queries/<uuid>` para el estado y `GET …/traffic_flows_async/queries/<uuid>/download` para el resultado. El endpoint síncrono `traffic_flows/traffic_analysis_queries` está obsoleto.

El análisis también necesita el inventario actual: `wkld-export`, `label-export`, `label-dimension-export` y, opcionalmente, `ipl-export --output-file pce-iplists.csv` (inicio rápido, paso 5). Columnas del export (24.x/25.x, una columna por tipo de etiqueta; cabecera completa en `docs/export-columns.txt`): `Source IP`, `Source Name`, `Source Hostname`, `Source Enforcement`, `Source App/Env/Loc/Role` (y cualquier tipo adicional), `Source FQDN`, las mismas para `Destination *`, `Port`, `Protocol`, `Process`, `Username`, `Num Flows`, `Bytes In`, `Bytes Out`, `Connection State`, `Reported Policy Decision`, `Reported by`, `First Detected`, `Last Detected`. Las columnas de etiquetas son las que separan el tráfico entre workloads gestionados del tráfico hacia IPs sin VEN.

## Objetos de política y convención de nombres

- **Workloads no gestionados (UMWL).** Entidades de red sin VEN que se dan de alta en el PCE para poder escribir reglas sobre ellas; la política entre un workload con VEN y uno no gestionado se aplica con las reglas del lado que tiene VEN. **Un peer se modela como UMWL cuando es un servidor concreto con rol identificable** (resolutores DNS, NTP internos, servidor y proxies Zabbix, indexadores Splunk, NetBackup master/media, bases de datos, VIP de balanceadores o front-ends, bastiones, escáneres de vulnerabilidades, relays SMTP, IBM MQ, OEM). **Uno por IP**, con etiquetas; aparece en las reglas de ringfence como cualquier otro workload. En la consola es *Workloads → Add → Add Unmanaged Workload*.
- **Listas de IP (IP lists).** Colecciones estáticas de direcciones, rangos y FQDN, para **peers amplios o que no son servidores**: VLAN de usuarios, subredes completas de una aplicación, metadata de nube (`169.254.169.254`, que en OCI es además el resolver de la VCN), NTP público, consolas SaaS de EDR, rangos publicados por un proveedor. También conviven con los UMWL como atajo: una lista `IPL_CDLV_OracleDB` permite escribir hoy la regla "app → Oracle 1521" y migrarla a etiquetas después.
- **Etiquetas y tipos de etiqueta.** Los cuatro tipos por defecto son Role, Application, Environment y Location (RAEL); el PCE admite tipos adicionales. Illumio aplica OR entre valores del mismo tipo y AND entre tipos distintos. **workloader crea valores de etiqueta que no existen, pero no crea tipos**: una columna del CSV que no es un tipo del PCE la ignora `wkld-import` en silencio (umwl-tui lo detecta en el paso 3 y ofrece descartarla o salir para crear el tipo con `label-dimension-import`).
- **Servicios.** Objetos de puerto/protocolo (`SVC_DNS`, `SVC_App_8080`). El informe los propone, pero **el kit no los crea**; se cargan con `workloader svc-import` o desde la consola cuando se escriben las reglas.

| Campo | Convención | Ejemplo |
|---|---|---|
| Etiquetas | Prefijo por tipo: `R_<Rol>`, `A_<App>`, `E_<Entorno>`, `L_<Ubicación>` | `R_DNS`, `A_CoreInfra`, `E_Prod`, `L_OCI` |
| `name` | `<rol descriptivo> <IP>`; es lo que se ve en la consola y lo que hace única a cada fila | `Zabbix Server 10.43.43.21` |
| `hostname` | **Vacío**. Los FQDN de los exports pueden estar pseudonimizados; un hostname falso confunde más de lo que ayuda. La pasada de creación hace match por `name` (`--match name`). | |
| `interfaces` | `eth0:<ip>` (la consola lo muestra como *eth0: 10.1.1.1*); varias interfaces separadas por `;` | `eth0:10.43.43.21` |
| `description` | Comentario del análisis con el prefijo `[<grupo> P<prioridad> conf:<Alta\|Media\|Baja>]`; la prioridad es la que filtra `--priority` | `[C3 P1 conf:Alta] Servidor Zabbix: consulta 10050/TCP…` |
| `review` | Columna de trabajo (PENDING, UPDATED, UNCHANGED; umwl-tui escribe NEW, EXISTS-UNMANAGED…); workloader la ignora | |
| Listas de IP | `IPL_<Sitio>_<Uso>`; la descripción empieza con `[<grupo>]` y termina con el uso en reglas | `IPL_CDLV_UserVLAN` |

## Contrato CSV de workloader

### `wkld-import` (workloads)

Cabeceras reconocidas (el resto se ignora): `href`, `hostname`, `name`, `interfaces`, `public_ip`, `distinguished_name`, `spn`, `enforcement`, `visibility`, `description`, `os_id`, `os_detail`, `data_center`, `external_data_set`, `external_data_reference` y **una columna por tipo de etiqueta** (`role`, `app`, `env`, `loc`, y los tipos personalizados que existan en el PCE).

```
hostname,name,interfaces,description,role,app,env,loc,review
,Zabbix Server 10.43.43.21,eth0:10.43.43.21,[C3 P1 conf:Alta] Servidor Zabbix…,R_Monitoring,A_Observability,E_Prod,L_CDLV,PENDING
```

- `interfaces`: `192.168.200.20`, `192.168.200.20/24`, `eth0:192.168.200.20` o `eth0:192.168.200.20/24`, separados por `;`.
- **Matching**: workloader hace match por **una** columna, elegida con `--match` (`href|hostname|name|external_data`) o, sin el flag, por prioridad entre las columnas presentes: `href`, después `hostname`, después `name`. Las filas cuya columna de match está vacía se descartan (`the match column cannot be blank`); no hay fallback de hostname a name. **Nunca por IP.** De ahí los flags del kit — `--match name` en la pasada de creación (hostnames vacíos) y `--match href` en la de actualización — y la reconciliación por IP: sin ella, un UMWL que ya existe con otro nombre se crea duplicado.
- `--umwl`: crea workloads no gestionados cuando el host no existe (se desactiva si se hace match por href). `--update` (por defecto `true`): actualiza los existentes; `--update=false` solo crea. `--allow-enforcement-changes`: necesario para tocar `enforcement`/`visibility`. `--max-create` / `--max-update`: tope de seguridad (-1 = sin límite).
- Sin `--update-pce`, el comando es un **dry run**: no escribe nada y deja en `workloader.log` (o `--log-file`) qué crearía y qué cambiaría. Con `--update-pce` pide confirmación; `--update-pce --no-prompt` no la pide (umwl-tui ya preguntó).

### `ipl-import` (listas de IP)

```
name,description,include,exclude,fqdns
IPL_CDLV_DNS,[C4] Resolutores corporativos — uso: reglas DNS,192.168.161.92;192.168.161.104;192.168.161.105,,
```

- `include` / `exclude`: IPs, CIDR o rangos separados por `;`. `fqdns` para nombres.
- Matching por `href` y, si no hay, por `name`: si el nombre existe, actualiza; si no, crea. `--ignore-href` para reutilizar un export de otro PCE. `--provision` (`-p`) provisiona después de crear/actualizar. Sin `--update-pce` es dry run.

## Anonimización

Los exports traen hostnames, FQDN, usuarios y a veces el nombre del cliente. `anonymize_export.py` los reemplaza de forma consistente (misma entrada → mismo token, en todas las columnas y en todas las corridas que comparten el mapa) y reversible; las IPs privadas, puertos, protocolos, procesos, valores de etiqueta, conteos y fechas se conservan porque son la evidencia.
```bash
python3 kit/anonymize_export.py anon TrafficData.csv -o TrafficData.anon.csv --map anon-map.json \
    --customer "Cliente A" --customer acme --domain cliente-a.com --domain corp.acme.local [--public-ips]
python3 kit/anonymize_export.py deanon C4-umwl-import.csv -o cliente-a-umwl-import.csv --map anon-map.json
```

`anon` mapea las columnas Source/Destination Name, Hostname y FQDN a tokens del estilo `host-0001.company.com` (los dominios a `company.com` / `dept.company.com` para que los tiers sigan reconocibles), los usuarios a `user-01…` (las cuentas de servicio conocidas como `root`, `oracle`, `zabbix` se conservan), los nombres de `--customer` a "Cliente", y las IPs públicas a `203.0.113.x` / `198.51.100.x` solo con `--public-ips`; termina con una verificación de fugas. `deanon` aplica el mapa inverso a cualquier CSV, así los workloads propuestos llevan los nombres reales en sus descripciones antes de cargarlos. `anon-map.json` (modo 0600, en `.gitignore`) es el secreto: nunca sale de la carpeta de trabajo. La guía (§5) cubre qué anonimizar y qué adjuntar.

## Análisis con Claude: instrucciones del Proyecto y prompt

El análisis se reproduce con un Proyecto de Claude cuyas instrucciones son `docs/prompts/context.md` y cuyo mensaje por conversación es `docs/prompts/prompt-short.md` (variantes v1 y v2); adjuntá el export pseudonimizado y el inventario `pce-*.csv`. Fuera de un Proyecto, adjuntá `context.md` y empezá con "Follow context.md". El prompt no se duplica acá (los entregables están en español porque así se usa el kit con clientes de LATAM; cambiá el idioma en `context.md` §0 si hace falta); en resumen pide:

1. Trabajar solo con los archivos adjuntos; citar los flujos detrás de cada inferencia; verificar los hechos externos contra documentación oficial; puerta de calidad antes de responder (conteos consistentes, sin roles derivados de hostnames, sin puertos inventados, CSV que parsean con las cabeceras exactas y names únicos).
2. Normalizar primero: IPs mutiladas por Excel, formatos de fecha mezclados, truncamiento en el tope de la consulta, poblaciones (VEN en alcance, hosts de laboratorio, flow logs de nube, escáneres), vocabulario de decisiones de política.
3. Clasificar cada peer sin VEN: UMWL (uno por IP, rol de servidor identificable) o lista de IP (VLAN, subredes, metadata de nube, SaaS por FQDN); balanceadores solo para las VIP.
4. Modelo de etiquetas `R_/A_/E_/L_`, reutilizando los valores que ya están en el PCE; `A_` define el ringfence.
5. Hallazgos de seguridad `S-xx` con severidad, evidencia y la acción en la política de Illumio; IDs estables entre versiones.
6. Entregables: informe HTML autocontenido en español, orientado al cliente, sin el nombre del cliente, doce secciones con diagramas SVG inline; `<grupo>-umwl-import.csv` y `<grupo>-ipl-import.csv` en los contratos exactos de workloader de arriba; workbook XLSX opcional. Para una v2: conservar cada entrada y cada ID de hallazgo, marcar "solo v1" / "nuevo", entregar de nuevo los CSV **completos** con `review` = PENDING / UPDATED / UNCHANGED.

Después de un CSV v2, umwl-tui hace el resto: en el paso 4 las filas cuya IP ya existe salen como EXISTS-UNMANAGED y se actualizan por `href`; las nuevas se crean.

## Sin la TUI: comandos workloader equivalentes

Misma carpeta de trabajo, mismo `pce.yaml`; `reconcile_umwl.py` (legacy) hace la reconciliación por IP sin escribir:

```bash
./workloader pce-add --api-key --name cliente-a --fqdn pce.cliente-a.example --port 443 \
    --api-user api_xxxxxxxx --api-secret '…' --org 1 --disable-tls-verification false
./workloader wkld-export --output-file pce-workloads.csv                                    # inventario (antes, pce-list)
python3 kit/legacy/reconcile_umwl.py pce-workloads.csv cliente-a-umwl-import.csv            # -to-create / -existing / -conflicts
./workloader wkld-import cliente-a-umwl-import-to-create.csv --umwl --update=false --match name    # dry run -> workloader.log
./workloader wkld-import cliente-a-umwl-import-existing.csv --match href                    # solo etiquetas/descripción, dry run
./workloader ipl-import cliente-a-ipl-import.csv                                            # dry run
# los mismos tres comandos con --update-pce para escribir (agregá --no-prompt para saltar la confirmación de workloader)
```

`reconcile_umwl.py` deja junto al CSV propuesto: `-to-create.csv` (IPs que no existen), `-existing.csv` (IPs de UMWL existentes, con `href` y `hostname` reescritos para que `wkld-import --match href` solo actualice etiquetas y descripción), `-conflicts.csv` (IPs de workloads con VEN o compartidas: decidir a mano) y `-reconcile-report.txt`.

## Legacy: loaders de terminal plana

`legacy/umwl_loader.py` (interactivo, prompts secuenciales, los mismos diez pasos) y `legacy/reconcile_umwl.py` (reconciliación no interactiva) fueron la primera implementación, en Python 3 con solo la biblioteca estándar. Se conservan para entornos donde el binario de Go no puede correr y como referencia legible; usan los mismos flags de workloader que umwl-tui (`pce-add --api-key`, `--match name` / `--match href`, la regla de dry run fallido), pero las funciones nuevas llegan primero a umwl-tui. Se ejecutan desde la carpeta de trabajo: `python3 kit/legacy/umwl_loader.py --setup-only`, y después `python3 kit/legacy/umwl_loader.py cliente-a-umwl-import.csv --ipl cliente-a-ipl-import.csv --priority 1` (mismos flags que umwl-tui). Detalles en [legacy/README.md](legacy/README.md).

## Solución de problemas

| Síntoma | Causa y solución |
|---|---|
| macOS no deja ejecutar `umwl-tui` o `workloader` ("no se puede abrir porque no se pudo verificar el desarrollador") | Cuarentena de Gatekeeper al descargar. `xattr -d com.apple.quarantine ./umwl-tui` (umwl-tui lo hace con workloader cuando lo descarga). |
| `workloader` en Apple Silicon | El release de macOS es un binario Intel y corre con Rosetta 2 (`softwareupdate --install-rosetta` si no está instalado). Para un binario nativo: `brew install go`; `umwl-tui --setup-only` ofrece clonar y compilar. |
| El dry run termina con `the match column cannot be blank` en todas las filas y `nothing to be done` | workloader hace matching por una columna, elegida con `--match` o, sin el flag, por prioridad entre las columnas presentes (href, hostname, name); con la convención de hostname vacío elige `hostname` y descarta todas las filas. umwl-tui y el loader legacy pasan `--match name` en la pasada de creación y `--match href` en la de actualización, y tratan ese dry run como fallido. A mano: `wkld-import to-create.csv --umwl --update=false --match name`. |
| `pce-add` pide correo y contraseña aunque pasaste `--api-user` | Falta el flag `--api-key`; sin él workloader ignora `--api-user/--api-secret/--org`. umwl-tui lo pasa siempre. |
| 401/403 al importar; el dry run funciona | La API key hereda el rol del usuario. Creala con un usuario Global Organization Owner o Global Administrator (un Workload Manager con alcance puede crear workloads, pero no etiquetas nuevas ni listas de IP), o usá una service account con esos permisos. Verificá también el `org id`. |
| Error TLS con PCE on-prem y certificado interno | `pce-add … --disable-tls-verification true` solo en laboratorio; en producción instalá la CA en el llavero. |
| El PCE que muestra `pce-list` no es el del cliente (u otra organización) | Carpeta equivocada (el directorio actual decide qué `pce.yaml` se usa) o `ILLUMIO_CONFIG` apunta a otro archivo. Una carpeta por cuenta + PCE; `unset ILLUMIO_CONFIG`; `--pce <nombre>` solo si el `pce.yaml` tiene varios perfiles. |
| El dry run actualiza un workload que no esperabas, o dos filas se cargan como una | Match por `name` con un objeto existente, o dos filas con el mismo valor de match. Revisá `runs/<ts>/dry-*.log`; renombrá la fila (paso 4/5) o dejá que la reconciliación por IP lo resuelva; umwl-tui agrega la IP como sufijo a los duplicados, a mano hacé único el `name`. |
| Una columna de etiqueta no se aplicó | El tipo de etiqueta no existe en el PCE; `wkld-import` ignora la columna. Crealo en la consola (Settings → Label Settings) o con `./workloader label-dimension-import tipos.csv --update-pce` (CSV con `key,display_name`), y volvé a correr. |
| `wkld-import` crea la etiqueta con otra capitalización | Los valores son sensibles a mayúsculas salvo `--ignore-case`. Usá exactamente los valores que muestra `label-export`. |
| Lote fallido en el paso 7 | Reintentar / saltar / abortar en el modal. Después revisá `runs/<ts>/create-chunkNNN.log`; el CSV de ese lote queda en `create-chunkNNN.csv` para reintentarlo a mano con `wkld-import … --umwl --update=false --match name --update-pce`. |
| El CSV propuesto todavía trae nombres `host-0012.company.com` | Se salteó la pasada `deanon`, o se corrió con otro `anon-map.json`. Volvé a correrla con el mapa de la carpeta donde corrió `anon`. |

## Guías (PDF/HTML)

Una guía unificada (30 páginas, mismo contenido en los dos idiomas) explica con diagramas el flujo de trabajo de un vistazo, los requisitos y la regla de una carpeta por cuenta + PCE, la inicialización, exportar desde el PCE con pantallas esquemáticas, la anonimización, el análisis con Claude, los objetos de política, umwl-tui paso a paso con sus puntos de decisión, umwl-tui y los subcomandos de workloader, el ciclo v2, la solución de problemas con una lista de control, y las columnas del export y los archivos del repositorio como apéndices.

- Español: [docs/Guia-umwl-tui-ES.pdf](docs/Guia-umwl-tui-ES.pdf) (HTML autocontenido: [docs/Guia-umwl-tui-ES.html](docs/Guia-umwl-tui-ES.html)).
- English: [docs/Guide-umwl-tui-EN.pdf](docs/Guide-umwl-tui-EN.pdf) (self-contained HTML: [docs/Guide-umwl-tui-EN.html](docs/Guide-umwl-tui-EN.html)).

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
