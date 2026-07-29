# MOVA · API de procesamiento de pagos (Go + Python)

Prueba técnica Backend — pista Go + Python. API de pagos desarrollada en Go más un proceso de conciliación en Python que
consume dicha API.

## Descripción

Servicio backend que permite a comercios registrar pagos, consultarlos,
actualizar su estado con trazabilidad completa, y obtener un resumen de
movimientos. Garantiza idempotencia en la creación de pagos y evita
condiciones de carrera bajo solicitudes concurrentes. Un proceso independiente
en Python concilia periódicamente los pagos `PENDING` con más de 30 minutos,
marcándolos como `REJECTED` a través de la propia API.

Toda la API queda detrás de autenticación JWT (ver sección "Autenticación"),
con idempotencia garantizada a nivel de base de datos para la creación de
pagos, y un resumen por comercio cacheado en Redis con invalidación explícita
ante cualquier cambio de estado.

## Arquitectura

```
cmd/
  api/                    # entrypoint del binario HTTP
internal/
  domain/                 # entidades y reglas de negocio puras (sin deps externas)
  application/            # casos de uso (orquestan dominio + repositorios)
  infrastructure/
    postgres/              # implementación de repositorios (sqlc + pgx)
    redis/                  # idempotencia rápida y cache del resumen
    provider/               # proveedor de pagos externo (simulado)
  transport/http/          # handlers Fiber, DTOs de request/response
  middleware/              # RequireAuth: validación de JWT en rutas protegidas
migrations/               # migraciones SQL versionadas
reconciliation/           # worker de conciliación en Python (cliente + script)
docs/                     # especificación OpenAPI
scripts/                  # utilidades de desarrollo
```

Separación de capas al estilo *clean architecture*: `domain` no depende de
nada externo (ni de Fiber ni de Postgres); `application` depende solo de
interfaces de dominio; `infrastructure` y `transport` son los únicos que
conocen detalles concretos (Fiber, SQL, Redis).

### Diagrama de arquitectura

```mermaid
flowchart TB
    subgraph Consumers["Consumidores"]
        direction LR
        CURL["Cliente HTTP<br/>(Postman / curl)"]
        PY["Worker de conciliación<br/>(Python)"]
    end

    MW["middleware.RequireAuth<br/>(valida JWT)"]

    subgraph Transport["Transport (HTTP)"]
        direction LR
        AuthH["AuthHandler"]
        MerchH["MerchantHandler"]
        PayH["PaymentHandler"]
    end

    subgraph Application["Application"]
        direction LR
        MerchS["MerchantService"]
        PayS["PaymentService"]
    end

    Domain["Domain<br/>Merchant · Payment · PaymentStatusHistory · Money"]

    subgraph Infra["Infrastructure"]
        direction LR
        PgRepo["postgres.*Repository"]
        RedisLock["redis.IdempotencyLocker"]
        RedisCache["redis.SummaryCache"]
        JWTAuth["infrastructure/auth"]
    end

    PG[("PostgreSQL")]
    RD[("Redis")]

    CURL --> MW
    PY --> MW
    MW --> AuthH
    MW --> MerchH
    MW --> PayH
    AuthH --> JWTAuth
    MerchH --> MerchS
    MerchH --> PayS
    PayH --> PayS
    MerchS --> Domain
    PayS --> Domain
    MerchS --> PgRepo
    PayS --> PgRepo
    PayS --> RedisLock
    PayS --> RedisCache
    PgRepo --> PG
    RedisLock --> RD
    RedisCache --> RD
```

- **Consumidores:** quien llama a la API — un cliente HTTP manual
  (Postman/curl) o el worker de conciliación en Python.
- **Transport (HTTP):** handlers Fiber + DTOs de request/response;
  mapea errores de las capas de abajo a códigos HTTP.
- **Application:** casos de uso (`MerchantService`, `PaymentService`) y
  los puertos (interfaces) que necesitan de la infraestructura.
- **Domain:** `Merchant`, `Payment`, `PaymentStatusHistory`, `Money` —
  sin ninguna dependencia externa (ni Fiber, ni SQL, ni Redis).
- **Infrastructure:** adaptadores concretos — repositorios Postgres
  (`sqlc` + `pgx`), lock/cache de Redis, emisión/validación de JWT.

`MerchantHandler` depende también de `PaymentService` porque el resumen
por comercio (`GET /merchants/{id}/summary`) es, de fondo, un cálculo
sobre pagos — el caso de uso vive en `PaymentService` aunque la ruta
cuelga de `/merchants` (ver Decisiones técnicas).

### Diagrama entidad-relación

```mermaid
erDiagram
    MERCHANTS ||--o{ PAYMENTS : "tiene"
    PAYMENTS ||--o{ PAYMENT_STATUS_HISTORY : "registra"

    MERCHANTS {
        uuid id PK
        text name
        text document_number UK
        text email
        text status
        timestamptz created_at
        timestamptz updated_at
    }

    PAYMENTS {
        uuid id PK
        uuid merchant_id FK
        text external_reference
        numeric amount
        text currency
        text payment_method
        text status
        text idempotency_key UK
        text provider_reference UK
        text provider_name
        timestamptz created_at
        timestamptz updated_at
    }

    PAYMENT_STATUS_HISTORY {
        uuid id PK
        uuid payment_id FK
        text previous_status
        text new_status
        text reason
        text changed_by
        timestamptz created_at
    }
```

`payments` tiene además una restricción única **compuesta**
`(merchant_id, external_reference)` — no representable como una sola
columna `UK` en el diagrama — que impide que el mismo comercio registre
dos veces la misma referencia externa (ver "Idempotencia y
concurrencia"). Ver `migrations/0001_init_schema.up.sql` para el DDL
completo.

### Patrones de diseño utilizados

- **Arquitectura hexagonal (Ports & Adapters):** el patrón estructural
  central de todo el proyecto — ver más abajo, en Decisiones técnicas,
  la justificación completa.
- **Repository:** `MerchantRepository`, `PaymentRepository`,
  `PaymentStatusHistoryRepository` (`internal/application/ports.go`) son
  interfaces que `application` define y `internal/infrastructure/postgres`
  implementa — el caso de uso nunca sabe que hay SQL detrás.
- **Adapter:** `internal/infrastructure/postgres` y `.../redis` adaptan
  librerías externas (`pgx`, `go-redis`) a los puertos que `application`
  espera, sin filtrar esos detalles hacia arriba.
- **Null Object:** `NoopIdempotencyLocker`/`NoopSummaryCache`
  (`internal/application/ports.go`) implementan los mismos puertos que
  sus versiones de Redis, pero "no hacen nada" (nunca consiguen el lock,
  siempre fallan el cache) — `PaymentService` no necesita ningún `if
  redis != nil` para funcionar sin Redis.
- **Cache-Aside:** `SummaryCache.Get` → miss → `PaymentService` calcula
  contra Postgres → `Set` — con invalidación explícita al cambiar el
  estado de un pago (ver "Idempotencia y concurrencia").
- **DTO (Data Transfer Object):** los structs de request/response en
  `internal/transport/http` (`paymentResponse`, `createPaymentRequest`,
  etc.) son deliberadamente distintos de las entidades de `domain` — el
  formato JSON de la API puede cambiar sin tocar una sola regla de
  negocio.
- **Inyección de dependencias por constructor:** cada `NewXxxService`/
  `NewXxxHandler`/`NewXxxRepository` recibe sus dependencias como
  parámetros de interfaz (ver el ensamblado completo en
  `cmd/api/main.go`); nada se instancia a sí mismo por dentro, lo que
  hace posible sustituir cualquier pieza por un doble de prueba.
- **Middleware (Chain of Responsibility):** `middleware.RequireAuth` se
  antepone a cada handler protegido — o deja pasar la petición
  (`c.Next()`) o la corta con `401`, sin que el handler final sepa nada
  de JWT.
- **Value Object:** `domain.Money` (monto + moneda) es inmutable, se
  valida a sí mismo al construirse, y no tiene una identidad propia más
  allá de su valor — dos `Money` con el mismo monto y moneda son
  intercambiables.
- **Máquina de estados explícita:** `domain.allowedTransitions` (un
  mapa de transiciones permitidas) + `CanTransition`, en vez de una
  cadena de `if/else` dispersa por el código, para las reglas de
  `PaymentStatus`.
- **Unit of Work:** `application.UnitOfWork` (patrón descrito por Martin
  Fowler en *Patterns of Enterprise Application Architecture*) agrupa
  varias escrituras en una sola transacción atómica — `PaymentService`
  solo ve el puerto (`Execute(ctx, fn)`), nunca un `*sql.Tx`; la
  implementación real vive en
  `internal/infrastructure/postgres/unit_of_work.go` — ver "Sección 2".

## Requisitos

- Go 1.25+
- Docker y Docker Compose
- PostgreSQL 16 (via Docker Compose, no requiere instalación local)
- Redis 7 (via Docker Compose, no requiere instalación local)
- Python 3.11+ (solo para el worker de conciliación)
- **En Windows:** los scripts de `scripts/` (`migrate.sh`,
  `sqlc-generate.sh`) son scripts de bash — hay que ejecutarlos desde
  **Git Bash** o WSL, no desde `cmd.exe` ni PowerShell directamente
  (ninguno de los dos interpreta bash). El resto de comandos de esta
  guía (`docker`, `curl`, `go`) funcionan igual en cualquier terminal.

## Instalación

```bash
git clone <url-del-repo>
cd Prueba_tecnica_GO-Python
cp .env.example .env
```

`.env.example` solo trae los nombres de las variables (sin valores), con un
comentario de ejemplo sobre cada una. **Antes de ejecutar el proyecto es
obligatorio completar `.env`** con valores propios (pueden ser inventados
para desarrollo local, ej. `DB_USER=mova`, `DB_PASSWORD=mova`), ya que no hay
valores por defecto embebidos en el código ni en todos los servicios de
`docker-compose.yml`.

## Variables de entorno

Ver [`.env.example`](.env.example). Resumen:

| Variable | Descripción |
|---|---|
| `PORT` | Puerto HTTP de la API |
| `SWAGGER_PORT` | Puerto de Swagger UI (documentación interactiva), por defecto `8081` |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | Conexión a PostgreSQL |
| `DATABASE_URL` | Cadena de conexión completa (derivada de las anteriores) |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB` | Conexión a Redis |
| `JWT_SECRET` | Secreto para firmar/validar tokens JWT (solo lo conoce el servidor, nunca se comparte) |
| `JWT_EXPIRATION_MINUTES` | Duración del token emitido |
| `AUTH_USERNAME`, `AUTH_PASSWORD` | Única credencial de servicio aceptada por `POST /api/v1/auth/login` (ver sección "Autenticación"); también las usa el worker de Python |
| `API_BASE_URL`, `RECONCILIATION_THRESHOLD_MINUTES` | Solo para el worker de conciliación en Python (ver esa sección) |

## Ejecución

```bash
docker compose up --build
```

Esto levanta la API, PostgreSQL y Redis. La API queda disponible en
`http://localhost:8080`.

**Sin Docker** (con Postgres ya levantado y migrado, ver siguiente sección):

```bash
go run ./cmd/api
```

Lee `PORT` y `DATABASE_URL` del `.env` (vía `godotenv`) o de variables de
entorno ya exportadas; `PORT` por defecto es `8080` si no se define.

## Documentación de la API

La especificación completa de todos los endpoints (comercios, pagos,
autenticación) está en [`docs/openapi.yaml`](docs/openapi.yaml) (OpenAPI
3.0): parámetros, request/response bodies, y todos los códigos de error
posibles por endpoint.

`docker compose up` levanta también una instancia de Swagger UI ya
apuntando a ese archivo — con todo corriendo, ábrela directamente en
`http://localhost:8081` (puerto configurable con `SWAGGER_PORT` en el
`.env`) para explorarla de forma interactiva, sin copiar/pegar nada en
ningún sitio externo.

## Migraciones

Las migraciones son archivos SQL versionados en `migrations/`, con un par
`.up.sql`/`.down.sql` por cada paso (`0001_init_schema` crea el esquema
inicial: `merchants`, `payments`, `payment_status_history`;
`0002_add_payment_provider_reference` agrega la columna
`provider_reference`; `0003_add_payment_provider_name` agrega
`provider_name` — ver "Sección 2"). Cada `.down.sql` revierte
exactamente lo que su `.up.sql` creó. Se aplican con
[`golang-migrate`](https://github.com/golang-migrate/migrate) a través de su
imagen Docker oficial, sin necesidad de instalar nada localmente.

Con Postgres ya levantado (`docker compose up -d postgres`) y el `.env`
completo (en Windows, corre esto desde **Git Bash**, ver "Requisitos"):

```bash
scripts/migrate.sh up          # aplica todas las migraciones pendientes
scripts/migrate.sh down 1      # revierte la última migración aplicada
scripts/migrate.sh version     # muestra la versión actual del esquema
```

El script conecta el contenedor de `migrate` a la misma red Docker
(`mova`) que usa `docker-compose.yml`, y arma la cadena de conexión interna
usando `postgres` (el nombre del servicio) como host — no `localhost`, que
solo es válido para herramientas que corren directamente en tu máquina.

## Pruebas

```bash
go test ./... -v
```

Por ahora cubre `internal/domain` (22 tests): creación válida de
`Merchant`/`Payment`, cada validación individual fallando (nombre vacío,
email inválido, monto ≤ 0, moneda no soportada, método de pago inválido,
referencia externa/llave de idempotencia faltantes), la tabla completa de
transiciones de estado válidas e inválidas (incluyendo `PROCESSING`/
`UNKNOWN` — ver "Sección 2"), y el caso de no poder volver a `PENDING`
desde un estado terminal. Son pruebas unitarias puras, sin base de datos
ni HTTP de por medio.

Los repositorios de Postgres (`internal/infrastructure/postgres`) tienen
pruebas de integración contra una base real, separadas con el build tag
`integration` para que no corran en el `go test ./...` normal (no todos los
entornos tienen Postgres levantado). Para correrlas:

```bash
docker compose up -d postgres
scripts/migrate.sh up
set -a && source .env && set +a
go test -tags=integration ./internal/infrastructure/postgres/... -v
```

Cubren: creación y lectura de comercios y pagos, `ErrNotFound` cuando el
recurso no existe, `ErrConflict` cuando se viola una restricción única
(`document_number` duplicado, `idempotency_key` duplicada,
`merchant_id + external_reference` duplicados, y `provider_reference`
duplicada), actualización de estado, `MarkProcessing` (guarda
`provider_reference` + paso a `PROCESSING`), creación/listado del
historial de estados, **listado con filtros y paginación** (por
comercio, por estado, con `page`/`limit`), la concurrencia real de 20
goroutines contra Postgres con la misma `Idempotency-Key`, y las 2
pruebas de atomicidad de `UnitOfWork` (revierte todo ante un fallo a
mitad de camino, confirma todo junto en el camino feliz — ver
"Sección 2"). 20 tests en total.

`internal/application` (29 tests) prueba `MerchantService` y
`PaymentService` con repositorios falsos en memoria: que el dominio
valide antes de persistir, que los errores se propaguen o se resuelvan
según corresponda, transición de estado válida/inválida con su registro
en el historial, que el resumen se calcule bien y use/invalide la cache
correctamente, que 20 goroutines concurrentes con la misma
`Idempotency-Key` converjan en un solo pago (ver "Idempotencia y
concurrencia" más abajo), el flujo con el proveedor de pagos (cobro
aprobado/rechazado, proveedor inalcanzable → `UNKNOWN`, y que un
reintento con la misma key concilie en vez de volver a cobrar), y los 5
escenarios de riesgo narrados con `t.Logf` — ver
"Sección 2".

`internal/infrastructure/auth` (4 tests) prueba la emisión y validación
de JWT en aislamiento: ida y vuelta con un secreto correcto, rechazo con
secreto equivocado, rechazo por expiración, rechazo por token malformado.
`internal/infrastructure/provider` (10 tests) prueba el proveedor de
pagos simulado (sus 4 comportamientos configurables, la referencia
desconocida, y que su mapa interno sea seguro bajo 20 goroutines
concurrentes) y el `Registry` que resuelve proveedores por nombre
(obtener uno registrado, error si no existe, y que varios proveedores
coexistan sin pisarse) — ver "Sección 2".
`internal/middleware` (5 tests) prueba `RequireAuth` como middleware
Fiber genérico (sin el router real de por medio): sin header, header sin
`Bearer`, token inválido, token expirado, token válido.
`internal/transport/http` (35 tests) prueba **todos** los endpoints de
comercios y pagos (`POST`/`GET`/`PATCH .../status`/`POST .../reconcile`/
`GET .../history`/`GET .../summary`) completos con `app.Test(...)` de
Fiber (request/response JSON reales, sin red), cubriendo los distintos
códigos HTTP posibles (`201`, `200`, `404`, `409`, `422`, `400`, `401`)
— incluye login válido/credenciales incorrectas, que las rutas reales
del router (no el middleware aislado) devuelvan `401` sin token/con
token inválido/expirado, que `/auth/login` sea la única ruta pública, y
que `/reconcile` resuelva un pago en `UNKNOWN` sin tocar uno ya
resuelto. Ninguno de estos paquetes necesita Postgres.

El worker de conciliación en Python (`reconciliation/`) tiene su propia
suite, independiente de `go test` — ver la sección "Worker de
conciliación (Python)" más abajo.

## Idempotencia y concurrencia

Requisito especial del documento base (sección 8): dos solicitudes
simultáneas con la misma `Idempotency-Key` nunca deben crear dos pagos.

### Estrategia de dos capas

1. **Restricción única en Postgres — la garantía real.**
   `UNIQUE (idempotency_key)` en `migrations/0001_init_schema.up.sql`.
   Sin importar qué pase en el resto del sistema (Redis caído, un bug en
   el código, mala suerte con los tiempos), Postgres físicamente no
   permite que dos filas compartan `idempotency_key`. Es la única pieza
   de la que depende la corrección.
2. **Lock opcional en Redis — una optimización, no una garantía.**
   `internal/infrastructure/redis/locker.go`. Antes de crear un pago,
   `PaymentService` intenta tomar un lock corto (`SET NX` + un token
   único + un script Lua para liberarlo sin pisar el lock de otro
   proceso). Si lo consigue, sigue directo. Si no, espera un instante
   corto, vuelve a chequear una sola vez, y sigue de todas formas si aún
   no hay nada — nunca bloquea indefinidamente. Sin Redis configurado
   (`REDIS_ADDR` vacío) o si falla, se usa `application.NoopIdempotencyLocker`
   (nunca "consigue" el lock): el sistema sigue siendo correcto, solo un
   poco menos eficiente bajo mucha concurrencia.

### El flujo completo (`internal/application/payment_service.go`)

1. Busca si ya existe un pago con esa `Idempotency-Key` → si sí, lo
   devuelve tal cual (replay).
2. Verifica que el comercio exista (si no, `404 MERCHANT_NOT_FOUND`).
3. Intenta el lock de Redis (best-effort, nunca bloqueante).
4. Valida con el dominio (`domain.NewPayment`) y persiste.
5. Si Postgres rechaza por conflicto, **distingue el motivo** volviendo a
   buscar por la `Idempotency-Key`: si aparece, la carrera fue por esa
   key → se devuelve el pago del ganador, sin error (idempotencia
   exitosa); si no aparece, el conflicto fue por
   `merchant_id + external_reference` duplicados → error real
   (`409 EXTERNAL_REFERENCE_ALREADY_EXISTS`). No todo "conflicto" de
   Postgres es un reintento legítimo.

### Pruebas

Dos tests de concurrencia, cada uno cubriendo un riesgo distinto:

- `TestPaymentService_Create_Concurrent` (`internal/application`): 20
  goroutines con la misma key contra un repositorio falso en memoria,
  usando a propósito `NoopIdempotencyLocker` (el lock de Redis "nunca
  ayuda"). Prueba que la **lógica de `PaymentService`** converge
  correctamente incluso en el peor caso — pero no detectaría una
  regresión en el esquema real de Postgres (el fake "finge" la
  restricción única con un mutex, sin importar qué diga la base real).
- `TestPaymentRepository_Create_ConcurrentSameIdempotencyKey`
  (`internal/infrastructure/postgres`): 20 goroutines reales contra
  Postgres, sin pasar por `PaymentService`. Prueba que la restricción
  única **de la base de datos** realmente existe y funciona — pero no
  detectaría un bug en la lógica de reintento de `PaymentService`
  (nunca pasa por ahí).

Ambos se corrieron repetidamente (20 y 10 veces seguidas respectivamente)
sin un solo fallo, para descartar que fuera casualidad.

## Autenticación

Todas las rutas de `/api/v1` requieren un JWT válido, **excepto**
`POST /api/v1/auth/login`.

### Flujo

1. `POST /api/v1/auth/login` con `{"username", "password"}` en el body.
2. El servidor compara ambos campos, en tiempo constante
   (`crypto/subtle.ConstantTimeCompare`, para no filtrar por diferencias
   de tiempo de respuesta cuántos caracteres acertó un intento), contra
   `AUTH_USERNAME`/`AUTH_PASSWORD` — la única credencial configurada, no
   hay tabla de usuarios.
3. Si coinciden, responde `200` con `{"token", "expires_at"}` — un JWT
   HS256 firmado con `JWT_SECRET`, con `sub=<username>` y `exp` según
   `JWT_EXPIRATION_MINUTES`. Si no, `401 INVALID_CREDENTIALS`.
4. El resto de endpoints exige el header
   `Authorization: Bearer <token>`. `internal/middleware.RequireAuth`
   valida firma y expiración; si falla por cualquier motivo (falta el
   header, no es `Bearer`, la firma no corresponde, expiró), responde
   `401 UNAUTHORIZED` sin distinguir el motivo exacto al cliente. Si es
   válido, deja el `sub` en `c.Locals` para que los handlers lo usen (ver
   `changed_by` en Decisiones técnicas).

### Por qué una sola credencial y no un sistema de usuarios

El JWT sigue siendo *stateless* (no hay sesiones ni tabla de tokens
activos: cualquier token con firma y expiración válidas se acepta) — la
única particularidad de este proyecto es que solo existe **una**
identidad posible detrás del token, la credencial de servicio
(`AUTH_USERNAME`), pensada para dos consumidores controlados por quien
despliega la API: quien la prueba manualmente (Postman/curl) y el worker
de conciliación en Python, que hace login programáticamente al arrancar
usando la misma credencial desde su propia configuración.

## Worker de conciliación (Python)

Proceso independiente, en `reconciliation/`, que **nunca toca la base de
datos directamente** — todo pasa por la API de Go, tal como exige el
enunciado. Es una sola pasada (no un loop continuo): se ejecuta, hace su
trabajo, imprime el resumen y termina; para correrlo periódicamente se
agenda externamente (cron, Task Scheduler, o similar).

### Instalación y ejecución

```bash
cd reconciliation
python -m venv .venv

# Windows
.venv\Scripts\activate
# Linux/Mac
source .venv/bin/activate

pip install -r requirements.txt
python reconciliation_worker.py
```

Lee la configuración del `.env` de la raíz del repo (no tiene uno propio):
reutiliza `AUTH_USERNAME`/`AUTH_PASSWORD` — la misma credencial de
servicio que usa la API, ver "Autenticación" — más dos variables nuevas,
`API_BASE_URL` (la URL de la API tal como la ve el worker, ej.
`http://localhost:8080`) y `RECONCILIATION_THRESHOLD_MINUTES` (opcional,
por defecto `30`).

### Qué hace, paso a paso

1. Login contra `POST /auth/login` (`client.py`).
2. Lista **todos** los pagos `PENDING`, recorriendo la paginación de
   `GET /payments` hasta agotar el `total` reportado (un comercio con más
   de 100 pagos pendientes no se corta a la primera página).
3. Filtra los que llevan más de `RECONCILIATION_THRESHOLD_MINUTES` desde
   su `created_at` (`filter_stale_payments`, función pura, sin HTTP).
4. Para cada uno, llama `PATCH /payments/{id}/status` con
   `status=REJECTED`. Si esa llamada falla (red o respuesta no exitosa),
   lo cuenta como fallido y **sigue con el resto** — un pago que no se
   pudo reconciliar no debe frenar a los demás.
5. Imprime el resumen exacto que pide el enunciado:
   ```
   Payments found: 5
   Payments reconciled: 4
   Payments failed: 1
   ```

Un fallo al autenticarse o al listar pagos pendientes sí es fatal (sin
eso no hay nada que hacer): imprime el error y termina con código de
salida `1`.

### Pruebas

```bash
cd reconciliation
pip install -r requirements-dev.txt
pytest -v
```

21 tests con `pytest` + `requests-mock` (la API de Go nunca corre de
verdad en estos tests):

- `test_config.py` (7): lectura de variables requeridas, default del
  umbral, error si falta alguna variable o si el umbral no es un entero.
- `test_client.py` (7): login exitoso/credenciales inválidas/error de
  red, paginación de `list_pending_payments` (una página y varias),
  `reject_payment` exitoso y con error del servidor.
- `test_reconciliation_worker.py` (5): la regla pura de "más de N
  minutos" (incluye el caso límite exactamente en el umbral), y `run()`
  con un cliente falso — reconciliación exitosa, un fallo que no detiene
  a los demás, y el caso sin nada que reconciliar.
- `test_main.py` (2): el flujo completo de `main()` con la API
  completamente mockeada (login, listado paginado, un rechazo que falla
  entre varios que sí funcionan) verificando el resumen impreso exacto, y
  que un fallo de login termine el proceso con código `1`.

Verificado también en vivo contra la API real en Docker: se creó un pago
`PENDING`, se corrió el worker con `RECONCILIATION_THRESHOLD_MINUTES=0`
(para no esperar 30 minutos de verdad) y se confirmó en
`GET /payments/{id}/history` que quedó `REJECTED` con
`changed_by: "mova-service"` — la misma credencial de servicio, porque el
worker se autentica igual que cualquier otro consumidor de la API. También
se probó con la API caída, confirmando que el mensaje de error de red es
claro y el proceso termina con código `1` en vez de quedar colgado o
fallar con un traceback críptico.

## Decisiones técnicas

- **Paginación con límite por defecto, nunca "todo" por defecto:**
  `GET /payments` sin `page`/`limit` no devuelve toda la tabla — usa
  `page=1, limit=100` (`application.DefaultPage`/`DefaultLimit`), y
  `limit` nunca puede superar `application.MaxLimit` (100), aunque se
  pida explícitamente. La razón: si "sin parámetros" significara "sin
  límite", un olvido del cliente (o un bug) podría traer miles de filas
  de una sola vez; el comportamiento por defecto de una API debe ser el
  seguro, no el más costoso. Quien necesite explorar más allá del límite
  usa `page` para paginar.
- **`changed_by` usa el subject del JWT:** `PATCH .../status` registra en
  el historial quién hizo el cambio. `transport/http.changedBy(c)` lee el
  subject que `middleware.RequireAuth` deja en `c.Locals` tras validar el
  token — como solo existe una credencial de servicio, el valor siempre es
  `"mova-service"` (o el `AUTH_USERNAME` configurado), pero queda aislado
  en una sola función a propósito: si en el futuro existiera un sistema
  de usuarios real, solo cambiaría esta función, no `PaymentService`.
- **Código de error `INVALID_PAYMENT_STATUS` para transición inválida:**
  se usa ese código exacto (no uno inventado) porque es literalmente el
  ejemplo que trae la sección 6.3 del documento base para este caso,
  respondido con `409 Conflict` (es un conflicto de estado, no un cuerpo
  de petición malformado) y el mensaje en español de
  `domain.ErrInvalidStatusTransition`, consistente con el resto del
  proyecto.
- **Arquitectura — puertos y adaptadores (hexagonal):** `internal/domain`
  no importa nada externo (ni Fiber, ni SQL, ni Redis); `internal/application`
  define únicamente las interfaces ("puertos") que necesita de la
  infraestructura (`MerchantRepository`, `PaymentRepository`, etc. en
  `ports.go`), sin saber si detrás hay Postgres, otra base, o un mock;
  `internal/infrastructure` (Postgres, Redis) y `internal/transport` (HTTP)
  son los "adaptadores" que implementan esos puertos o los consumen.
  - **Testabilidad:** los casos de uso se pueden probar contra un
    repositorio falso en memoria, sin levantar Postgres — las pruebas de
    reglas de negocio no dependen de infraestructura real.
  - **Reemplazabilidad:** cambiar de Postgres a otra base de datos, o
    agregar una capa de cache, solo toca `internal/infrastructure` — el
    dominio y los casos de uso no se enteran ni cambian.
  - **Protección del dominio:** las reglas de negocio quedan aisladas de
    detalles que cambian con el tiempo (versión de Fiber, del driver SQL,
    etc.), en vez de estar mezcladas con código de framework.
  - **`ErrNotFound`/`ErrConflict` viven en `application`, no en
    `infrastructure`:** originalmente estos dos errores genéricos
    vivían en `internal/infrastructure/postgres`, porque solo
    `transport/http` necesitaba reconocerlos — una capa "de arriba"
    conociendo detalles de una "de abajo" no rompe la regla. Eso cambió
    en el proceso de desarrollo: `PaymentService` (en `application`, la capa
    *intermedia*) necesita inspeccionar estos errores para decidir si
    hacer replay de idempotencia o reintentar una creación de pago. Si
    siguieran en `postgres`, `application` terminaría dependiendo de
    `infrastructure`, invirtiendo la flecha de dependencia. Por eso ahora
    viven en `internal/application/errors.go`, como parte del contrato
    del puerto: cualquier repositorio (Postgres, un fake de test, u otra
    base) debe devolver estos errores genéricos, y `postgres.mapError`
    los reutiliza en vez de definir los suyos.
  - **Regla para los dobles de prueba (fakes) en tests:** cuando el
    código bajo prueba **no inspecciona** el error, solo lo reenvía (ej.
    `MerchantService`), el fake puede usar un error inventado
    (`errors.New("...")`) — lo único que importa es que el servicio lo
    deje pasar sin tocarlo. Cuando el código **sí** decide en base al
    tipo de error (ej. `PaymentService`, que distingue "no existe" de
    "conflicto" para decidir si reintenta), el fake debe devolver los
    errores reales (`application.ErrNotFound`/`ErrConflict`), porque ahí
    la identidad del error es parte de la lógica que se está probando.
- **Framework HTTP:** Fiber, por ser el preferido según el anexo de la
  prueba y su bajo overhead sobre `fasthttp`.
- **Timestamps siempre en UTC en las respuestas:** los handlers formatean
  `CreatedAt`/`UpdatedAt` con `.UTC().Format(time.RFC3339)`, nunca solo
  `.Format(time.RFC3339)`. El motivo: un `time.Time` recién creado en el
  dominio ya está en UTC, pero el mismo valor leído de vuelta desde
  Postgres a través de `pgx` llega en la zona horaria local del proceso —
  incluso siendo el mismo instante exacto, se mostraba distinto según si
  el dato venía de un `Create` o de un `GetByID` (`...Z` vs. `...-05:00`).
  Forzar `.UTC()` al serializar deja el formato consistente sin importar
  el origen del dato.
- **Acceso a datos — por qué no un ORM:** se usa `sqlc` + `database/sql`
  (con `pgx` registrado como driver) en vez de un ORM como GORM.
  - El SQL se escribe a mano en `internal/infrastructure/postgres/queries/*.sql`
    y `sqlc` genera, a partir de ahí, structs y funciones Go type-safe —
    sin reflection en runtime y sin que arme queries dinámicamente
    por debajo. Lo que se ejecuta contra la base es exactamente el SQL que
    se escribe, verificable leyendo el archivo `.sql`.
  - Con un ORM es fácil terminar con problemas de rendimiento ocultos
    (ej. N+1 queries) o SQL generado subóptimo sin darse cuenta, porque el
    ORM abstrae la query real. Aquí, si una consulta es lenta o compleja,
    se ve y se optimiza directamente en el `.sql`.
  - `sqlc` sigue dando *type-safety* en tiempo de compilación (los
    parámetros y columnas devueltas son structs Go generados, no `map[string]interface{}`
    ni *interface{}* sueltos), que es la principal ventaja que suele
    atraer a un ORM — sin pagar el costo de la abstracción extra.
  - Costo aceptado: hay que escribir cada query a mano y regenerar el
    código (`scripts/sqlc-generate.sh`) cada vez que cambian; para el
    tamaño de este proyecto (pocas tablas, queries acotadas) es un costo
    bajo comparado con el control y la claridad que da.
  - Nota de configuración: para que `sqlc` mapee columnas `NUMERIC` a
    `decimal.Decimal` (ver punto de Dinero abajo), el override en
    `sqlc.yaml` debe usar `db_type: "pg_catalog.numeric"` — el nombre
    corto `"numeric"` no lo reconoce y la columna queda como `string`.
  - Segunda nota, misma causa raíz: en una **expresión calculada** (ej.
    `COALESCE(sum(amount) FILTER (...), 0)` del resumen por comercio),
    `sqlc` no puede inferir el tipo solo — sin un cast explícito
    (`::numeric`), el campo generado queda como `interface{}` en vez de
    `decimal.Decimal`. El override solo ayuda cuando `sqlc` ya sabe que
    está mirando una columna `NUMERIC`; una expresión hay que
    "etiquetarla" a mano.
- **Dinero — por qué `decimal.Decimal` y no `float64`:** columnas
  `NUMERIC` en PostgreSQL mapeadas a `shopspring/decimal` en Go.
  - Los `float64` usan coma flotante binaria (IEEE 754): la mayoría de
    los valores decimales "normales" (`0.10`, `19.99`, etc.) no tienen
    representación exacta en binario, igual que 1/3 no la tiene en
    decimal. Esto produce errores de redondeo reales, por ejemplo
    `0.1 + 0.2 == 0.30000000000000004` o `19.99 * 3 == 59.97000000000001`
    en la mayoría de los lenguajes, Go incluido.
  - Para dinero eso es inaceptable: sumar miles de transacciones con ese
    error acumulado descuadraría los totales del resumen por comercio
    frente a lo que realmente se procesó.
  - `decimal.Decimal` representa el número como un entero exacto más un
    exponente (`19.99` se guarda como `1999 × 10⁻²`), igual que `NUMERIC`
    en Postgres — sin paso por binario, sin error de redondeo. El costo
    (aritmética un poco más lenta que con floats nativos) es irrelevante
    para el volumen de este sistema.
- **Autenticación — por qué una sola credencial de servicio y no un
  sistema de usuarios:** el documento base pide "algún mecanismo de
  autenticación", sin exigir registro/roles/multiusuario. Implementar una
  tabla `users` completa (con hashing de contraseñas, gestión de cuentas,
  etc.) sería resolver un problema que el enunciado no plantea. En su
  lugar, `AUTH_USERNAME`/`AUTH_PASSWORD` (config, no base de datos) son la
  única credencial válida para `POST /api/v1/auth/login`; quien la
  presenta recibe un JWT (HS256, `sub`=username, `exp` según
  `JWT_EXPIRATION_MINUTES`) que debe enviarse como
  `Authorization: Bearer <token>` en el resto de endpoints de
  `/api/v1` — ver sección "Autenticación" más abajo para el detalle
  completo del flujo.
- **Redis:** usado para (1) lock rápido de idempotencia en la creación de
  pagos (ver sección "Idempotencia y concurrencia") y (2) cache del
  endpoint de resumen por comercio (`GET /merchants/{id}/summary`),
  invalidada explícitamente cuando cambia el estado de un pago de ese
  comercio — no solo dejada expirar por TTL (30s), para que el resumen
  nunca muestre datos desactualizados tras un cambio real. Ambos usos
  siguen el mismo principio: Redis es una optimización de mejor esfuerzo,
  nunca la fuente de verdad — si no está configurado o falla, el sistema
  sigue siendo correcto (`NoopIdempotencyLocker`/`NoopSummaryCache`),
  solo un poco más lento.
- **Worker de Python — por qué una sola pasada y no un loop continuo:**
  el enunciado lo ilustra como `python reconciliation_worker.py`, una
  invocación puntual con un resumen impreso al final — no como un
  servicio de larga duración. Una sola pasada es además más fácil de
  probar de forma determinista (sin manejar señales ni temporizadores) y
  más simple de operar: se agenda con las herramientas que ya existen
  para eso (cron, Task Scheduler), en vez de reinventar un scheduler
  propio dentro del script.
- **El worker nunca toca Postgres directamente:** todo pasa por la API
  de Go (`client.py` solo hace peticiones HTTP) — cumple la restricción
  explícita del enunciado, y de paso mantiene una sola fuente de verdad
  para las reglas de negocio (transición de estados, idempotencia): el
  worker no necesita — ni puede — saltárselas.

## Supuestos

El enunciado no detalla todo el comportamiento posible; donde tuvimos que
decidir, asumimos:

1. **Moneda única:** solo `COP` es válido hoy (`domain.SupportedCurrency`).
   Agregar otra moneda requeriría revisar `Money` y probablemente manejar
   tasas de conversión, fuera del alcance de esta prueba.
2. **Estados de comercio:** el enunciado no define estados para
   `Merchant`, así que se asumió `ACTIVE`/`INACTIVE`, arrancando siempre
   en `ACTIVE`.
3. **La llave de idempotencia la genera el cliente**, no la API — el
   servidor solo la valida y la usa para detectar reintentos.
4. **IDs:** UUID v4 en formato string para todas las entidades.
5. **Precisión monetaria:** `NUMERIC(18,2)` — hasta 2 decimales, aunque el
   ejemplo del enunciado usa montos enteros (`150000`).
6. **`document_number` es único globalmente** entre comercios (el
   enunciado no lo dice explícitamente, pero es la interpretación de
   negocio más razonable para un identificador fiscal).

## Limitaciones

- **Puerto de Postgres configurable por conflictos locales:** si en tu
  máquina ya corre un PostgreSQL nativo (ej. un servicio de Windows)
  escuchando en `5432`, el contenedor de este proyecto no podrá publicar
  ahí y las conexiones desde el host a `localhost:5432` llegarán al
  Postgres nativo en vez de al contenedor (fallando la autenticación con
  usuarios que solo existen en el contenedor). Solución: cambiar
  `DB_PORT` en tu `.env` a un puerto libre (ej. `5433`) y actualizar
  `DATABASE_URL` acorde — no requiere tocar código ni `docker-compose.yml`,
  ambos ya usan `DB_PORT` como variable. Esto solo afecta conexiones desde
  el host (tests de integración, clientes SQL); las conexiones entre
  contenedores (`scripts/migrate.sh`, la API corriendo en Docker) siempre
  usan el puerto interno `5432` y no se ven afectadas.
- **Puerto de la API configurable por el mismo motivo:** si en tu máquina
  ya hay otro proceso escuchando en `8080`, cambia `PORT` en tu `.env` a uno libre
  (ej. `18080`) — `docker-compose.yml` ya usa `${PORT:-8080}` para el
  mapeo del contenedor `api`, no requiere tocar código.
- **`docker-compose.yml` separa el `DATABASE_URL`/`REDIS_ADDR` del
  contenedor `api` de los que usa tu `.env`:** el `.env` usa
  `localhost:${DB_PORT}` porque así lo necesitan las herramientas que
  corren en tu máquina (tests de integración, `scripts/migrate.sh`); pero
  dentro de la red de Docker, "localhost" es el propio contenedor `api`,
  no el de Postgres/Redis. Por eso `docker-compose.yml` sobrescribe esas
  dos variables solo para el servicio `api`, apuntándolas al nombre del
  servicio (`postgres:5432`, `redis:6379`) — la única forma de que
  `docker compose up --build` funcione de punta a punta sin pasos
  manuales adicionales.

## Sección 2

Resiliencia frente a proveedores externos de pago (ej. Nequi): qué pasa
cuando la respuesta de una operación se pierde antes de que MOVA la
guarde, cómo se evita un doble cobro en un reintento, y cómo se concilia
después el estado real de una operación incierta.

### El riesgo: aprobación fantasma / estado incierto

Escenario concreto: un usuario inicia un pago, el proveedor externo lo
aprueba de verdad, pero **antes de que MOVA guarde esa respuesta, el
proceso se cae** (timeout, caída de red, reinicio). El usuario, sin saber
si su pago se procesó, reintenta. Los riesgos reales de este escenario:

- **Doble cobro**: si el reintento simplemente vuelve a llamar al
  proveedor sin verificar nada, el usuario paga dos veces por la misma
  operación — el proveedor no tiene forma de saber que es un reintento
  si no le enviamos alguna referencia que ya conociera.
- **Estado incierto indefinido**: sin un estado explícito para "no sé qué
  pasó", el pago quedaría atascado en `PENDING` como si nunca se hubiera
  intentado — perdiendo el hecho de que sí se llegó a contactar al
  proveedor.
- **Reconciliación tardía o inexistente**: sin un mecanismo para
  preguntarle al proveedor "¿qué pasó con esta operación?", un pago
  incierto nunca se resolvería solo — quedaría así para siempre, o peor,
  alguien terminaría decidiendo su estado a mano sin la certeza real.

### Estados nuevos: `PROCESSING` y `UNKNOWN`

Se amplió la máquina de estados de `domain.Payment`
(`internal/domain/payment.go`) con dos estados intermedios, sin quitar
ninguna transición existente (`PATCH /payments/{id}/status` sigue
funcionando exactamente igual para cambios manuales).

| Desde | Hacia | Cómo se llega |
|---|---|---|
| `PENDING` | `PROCESSING` | Automático: justo antes de llamar al proveedor externo |
| `PENDING` | `APPROVED` / `REJECTED` / `CANCELLED` | Manual, vía `PATCH /payments/{id}/status` (sin proveedor de por medio) |
| `PROCESSING` | `APPROVED` / `REJECTED` | El proveedor respondió con claridad (al momento de cobrar, o después al conciliar) |
| `PROCESSING` | `UNKNOWN` | El proveedor no respondió (timeout, caída de red) |
| `UNKNOWN` | `APPROVED` / `REJECTED` | Se resolvió conciliando con el proveedor |

Ninguna fila permite volver a un estado anterior (ej. `APPROVED → PENDING`)
ni saltarse un paso (ej. `PENDING → UNKNOWN` directo, sin pasar por
`PROCESSING`) — la tabla completa vive en `allowedTransitions`
(`internal/domain/payment.go`), y **`CANCELLED` no es alcanzable desde
`PROCESSING` ni desde `UNKNOWN` a propósito**: una vez que la operación
se envió al proveedor, "cancelar" ya no es una decisión que MOVA pueda
tomar unilateralmente — si el proveedor en realidad sí procesó el cobro,
marcarlo `CANCELLED` sería falsear el registro contable. Desde esos dos
estados, la única salida es preguntarle al proveedor qué pasó de verdad
(conciliación) y resolver a `APPROVED` o `REJECTED` según la respuesta
real — nunca decidir "cancelado" sin esa confirmación.

- **`PROCESSING`**: la operación ya se envió al proveedor externo, la
  respuesta todavía no se conoce. Se persiste **antes** de llamar al
  proveedor (no después) — así, si el proceso se cae a mitad de esa
  llamada, el pago queda con evidencia de que la operación quedó en
  camino, no como si nunca hubiera pasado nada.
- **`UNKNOWN`**: se le preguntó al proveedor y no hubo una respuesta
  clara (timeout, caída de red). No es un estado terminal — la única
  salida es conciliar con el proveedor (nunca una cancelación manual:
  cancelar algo que en realidad sí se cobró falsearía el registro).

### Identificador estable de operación (`provider_reference`)

Columna nueva en `payments` (migración
`0002_add_payment_provider_reference.up.sql`), única cuando existe,
`NULL` hasta que el pago se envía al proveedor. Es distinta de
`idempotency_key` (la genera el cliente de esta API) y de `id` (lo genera
este sistema) — `provider_reference` identifica la operación de cara al
proveedor externo.

Decisión importante: **la genera MOVA, no el proveedor**, y se persiste
junto con el paso a `PROCESSING` en una sola escritura, **antes** de
llamar al proveedor (`PaymentRepository.MarkProcessing`). La alternativa
— esperar a que el proveedor la devuelva en su respuesta — no serviría
para el escenario de riesgo: si la respuesta se pierde, nunca tendríamos
esa referencia para poder preguntar después. Generándola nosotros de
antemano, siempre queda algo estable para conciliar, sin importar qué
pase con la llamada al proveedor.

### La abstracción del proveedor (`PaymentProvider`)

Puerto nuevo en `internal/application/ports.go`, con solo dos
operaciones:

```go
type PaymentProvider interface {
    Charge(ctx context.Context, req ChargeRequest) (ProviderStatus, error)
    GetStatus(ctx context.Context, providerReference string) (ProviderStatus, error)
}
```

`ProviderStatus` tiene **tres** valores posibles (`APPROVED`, `REJECTED`,
`PROCESSING`) — el tercero cubre el caso legítimo de que el proveedor
mismo tampoco tenga todavía una respuesta definitiva al conciliar (no es
un error, es "vuelve a preguntar más tarde"). Un error dedicado,
`application.ErrProviderUnreachable`, distingue "no se pudo contactar al
proveedor en absoluto" de una respuesta real de rechazo.

La implementación (`internal/infrastructure/provider`, sin credenciales
reales de sandbox) es un simulador con 4 comportamientos configurables
por instancia, pensados para reproducir cada escenario de riesgo de forma
determinista en pruebas:

- `BehaviorApprove` / `BehaviorReject`: responde de inmediato.
- `BehaviorTimeout`: `Charge` falla con `ErrProviderUnreachable`, y el
  proveedor tampoco tiene ninguna verdad guardada al conciliar
  (`GetStatus` devuelve `PROCESSING`).
- `BehaviorApprovedButLost`: reproduce el escenario de riesgo central —
  el proveedor **sí aprueba de verdad** (lo guarda en su estado interno),
  pero `Charge` le devuelve a MOVA `ErrProviderUnreachable`, como si la
  respuesta se hubiera perdido. `GetStatus`, después, sí revela la
  aprobación real al conciliar — separar "lo que el proveedor sabe" de
  "lo que nos dijo" es lo que hace posible resolver este caso.

### Diseñado para más de un proveedor externo

El enunciado menciona Nequi como ejemplo, pero nada en el diseño depende
de que exista un solo proveedor — se construyó pensando en que en el
futuro pudiera haber varios (ej. Nequi y Bre-B) operando a la vez:

- **`provider_name`** (columna nueva en `payments`, migración
  `0003_add_payment_provider_name.up.sql`) registra **cuál** proveedor
  procesó cada pago — distinto de `provider_reference`, que identifica
  **la operación** dentro de ese proveedor. Ambos son necesarios: con
  varios proveedores posibles, conciliar requiere saber tanto la
  referencia como a *quién* preguntarle por ella.
- **`application.ProviderRegistry`** (puerto nuevo) resuelve, por
  nombre, qué `PaymentProvider` usar:
  ```go
  type ProviderRegistry interface {
      Get(providerName string) (PaymentProvider, error)
  }
  ```
  `PaymentService` nunca necesita un `if providerName == "nequi"` — solo
  le pide al registro el proveedor correspondiente, y usa la interfaz
  genérica sin saber cuál implementación concreta hay detrás.
- **`internal/infrastructure/provider/registry.go`** es la implementación
  real: un mapa en memoria donde `cmd/api/main.go` registra, una sola vez
  al arrancar, cada proveedor disponible (`registry.Register("nequi",
  nequiProvider).Register("bre-b", brebProvider)`). Pedir un proveedor no
  registrado devuelve `application.ErrUnknownProvider`, no un `nil`
  silencioso.

Ahora mismo solo existe un proveedor real registrado (el simulado) — el
mecanismo de *selección* (qué regla decide cuál proveedor usar para un
pago dado: ¿el método de pago? ¿una configuración por comercio?) todavía
no está definido, porque no tiene sentido diseñarlo sin un segundo
proveedor real contra el cual validarlo. Lo que sí queda resuelto desde
ya es que agregar ese segundo proveedor, el día que exista, no requiere
tocar `PaymentService` ni ninguna regla de negocio — solo escribir el
adaptador nuevo y registrarlo.

### Atomicidad: estado + historial en una sola transacción

Antes de este cambio, `PaymentService.UpdateStatus` escribía el nuevo
estado y la entrada de historial en dos llamadas independientes a
Postgres — sin ninguna transacción de por medio, una caída justo entre
ambas dejaría un pago con un estado que ningún registro de historial
explica.

Se agregó un puerto `UnitOfWork` (`internal/application/ports.go`):

```go
type UnitOfWork interface {
    Execute(ctx context.Context, fn func(ctx context.Context) error) error
}
```

La implementación real (`internal/infrastructure/postgres/unit_of_work.go`)
abre una transacción de Postgres, y los repositorios de ese mismo paquete
(`PaymentRepository`, `PaymentStatusHistoryRepository`) detectan si el
`context.Context` que reciben trae una transacción activa — si la trae,
la usan; si no, usan la conexión normal (ver `tx.go`). Así, `application`
nunca se entera de que existe un `*sql.Tx` por debajo, solo ve el puerto
`UnitOfWork`; y `PaymentService.UpdateStatus` queda:

```go
s.uow.Execute(ctx, func(txCtx context.Context) error {
    if err := s.payments.UpdateStatus(txCtx, payment); err != nil {
        return err
    }
    return s.history.Create(txCtx, history)
})
```

Verificado con dos pruebas de integración contra Postgres real
(`internal/infrastructure/postgres/unit_of_work_test.go`): una fuerza un
error justo después de la primera escritura y confirma que **ambas**
quedan revertidas (no solo la que falló), y otra confirma que el camino
feliz persiste las dos juntas.

### El flujo de creación completo, con el proveedor conectado

`PaymentService.Create` (`internal/application/payment_service.go`) ya
no solo persiste el pago en `PENDING` — lo envía de verdad al proveedor
configurado y resuelve el resultado antes de responder:

1. **Replay** por `Idempotency-Key`: si el pago ya existe y quedó en
   `PROCESSING`/`UNKNOWN` de un intento anterior, se concilia ahí mismo
   (ver más abajo) antes de devolverlo — **nunca se vuelve a llamar al
   proveedor** para una key repetida.
2. El comercio debe existir; lock opcional en Redis (igual que siempre).
3. Se valida con el dominio y se persiste en `PENDING`.
4. **`PENDING → PROCESSING`**: se genera un `provider_reference` nuevo y
   se guarda de forma atómica con su entrada de historial — antes de
   llamar al proveedor.
5. Se llama a `provider.Charge(...)`. Si responde claro, se resuelve a
   `APPROVED`/`REJECTED`. Si devuelve `ErrProviderUnreachable`, se marca
   `UNKNOWN`.

Un método público, `PaymentService.Reconcile(ctx, paymentID, changedBy)`,
le pregunta al proveedor el estado real de un pago en
`PROCESSING`/`UNKNOWN` — es el mismo mecanismo que usa el paso 1 del
replay, expuesto también como `POST /api/v1/payments/{id}/reconcile`
para que algo externo (ej. el worker de Python, ver más abajo) lo
dispare sobre pagos atascados por demasiado tiempo. Si el pago ya está
resuelto, es un no-op — nunca cambia nada ni falla.

Verificado en vivo contra la API real en Docker: un pago creado se
resuelve a `APPROVED` en la misma respuesta de `POST /payments`, con el
historial mostrando `PENDING→PROCESSING→APPROVED`, `changed_by` con el
subject del JWT autenticado, y `provider_reference`/`provider_name`
guardados en Postgres.

### Los 5 escenarios de riesgo, probados y narrados

`internal/application/payment_provider_scenarios_test.go` tiene 5 tests
dedicados, cada uno con `t.Logf` narrando paso a paso lo que pasa — se
corren así, con `-v` para ver la narración:

```bash
go test ./internal/application/... -run TestScenario -v
```

1. **`TestScenario1_NormalApproval`** — respuesta aprobada normal.
2. **`TestScenario2_TimeoutBeforeKnowingResult`** — el proveedor no
   responde, y conciliar de inmediato tampoco revela nada (a diferencia
   del escenario 4, acá el proveedor genuinamente no sabe todavía).
3. **`TestScenario3_RetrySameIdempotencyKey`** — reintento con la misma
   key; se verifica contando las llamadas reales a `provider.Charge`
   (debe quedar en `1`, nunca en `2`).
4. **`TestScenario4_ApprovedButResponseLost`** — el escenario de riesgo
   central del documento base: el proveedor aprueba de verdad, la
   respuesta se pierde, y la conciliación revela la verdad.
5. **`TestScenario5_ConcurrentRequests`** — 2 peticiones concurrentes con
   la misma key; una sola fila, un solo cobro real al proveedor.

### Diagrama de estados con el proveedor conectado

```mermaid
stateDiagram-v2
    [*] --> PENDING
    PENDING --> PROCESSING: se envía al proveedor
    PENDING --> APPROVED: PATCH manual
    PENDING --> REJECTED: PATCH manual
    PENDING --> CANCELLED: PATCH manual
    PROCESSING --> APPROVED: el proveedor aprueba
    PROCESSING --> REJECTED: el proveedor rechaza
    PROCESSING --> UNKNOWN: el proveedor no responde
    UNKNOWN --> APPROVED: la conciliación revela aprobación
    UNKNOWN --> REJECTED: la conciliación revela rechazo
    APPROVED --> [*]
    REJECTED --> [*]
    CANCELLED --> [*]
```

### Diagrama de secuencia — Escenario 4 (el riesgo central)

```mermaid
sequenceDiagram
    participant Cliente
    participant MOVA
    participant Proveedor

    Cliente->>MOVA: POST /payments (Idempotency-Key: abc-123)
    MOVA->>MOVA: Guarda PENDING, genera provider_reference
    MOVA->>MOVA: Marca PROCESSING (atómico con su historial)
    MOVA->>Proveedor: Charge(provider_reference, monto)
    Proveedor->>Proveedor: Aprueba internamente
    Proveedor--xMOVA: la respuesta se pierde en el camino
    MOVA->>MOVA: Charge devolvió error -> marca UNKNOWN
    MOVA-->>Cliente: 201 { status: "UNKNOWN" }

    Note over Cliente,Proveedor: Más tarde — se dispara la conciliación

    Cliente->>MOVA: POST /payments/{id}/reconcile
    MOVA->>Proveedor: GetStatus(provider_reference)
    Proveedor-->>MOVA: APPROVED (la verdad, revelada)
    MOVA->>MOVA: Marca APPROVED (atómico con su historial)
    MOVA-->>Cliente: 200 { status: "APPROVED" }
```

### Diagrama de secuencia — Escenario 5 (concurrencia)

```mermaid
sequenceDiagram
    participant A as Petición A
    participant B as Petición B
    participant MOVA
    participant Proveedor

    par Llegan al mismo tiempo
        A->>MOVA: POST /payments (misma Idempotency-Key)
    and
        B->>MOVA: POST /payments (misma Idempotency-Key)
    end

    Note over MOVA: UNIQUE(idempotency_key) en Postgres decide: solo UNA gana

    MOVA->>Proveedor: Charge(...) — solo la ganadora llama
    Proveedor-->>MOVA: APPROVED

    MOVA-->>A: 201 { id: "xyz", status: "APPROVED" }
    MOVA-->>B: 201 { id: "xyz", status: "APPROVED" } (mismo pago)
```
