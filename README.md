# MOVA · API de procesamiento de pagos (Go + Python)

Prueba técnica Backend — pista Go + Python. API de pagos desarrollada en Go
(75% de la calificación) más un proceso de conciliación en Python (25%) que
consume dicha API.

## Descripción

Servicio backend que permite a comercios registrar pagos, consultarlos,
actualizar su estado con trazabilidad completa, y obtener un resumen de
movimientos. Garantiza idempotencia en la creación de pagos y evita
condiciones de carrera bajo solicitudes concurrentes. Un proceso independiente
en Python concilia periódicamente los pagos `PENDING` con más de 30 minutos,
marcándolos como `REJECTED` a través de la propia API.

_(Esta sección se irá ampliando a medida que se implementen las reglas de
negocio — fases en curso, ver historial de commits.)_

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
  transport/http/          # handlers Fiber, DTOs de request/response
  middleware/              # auth JWT, logging, recuperación de panics
migrations/               # migraciones SQL versionadas
reconciliation/           # worker de conciliación en Python (cliente + script)
tests/                    # pruebas de integración transversales
scripts/                  # utilidades de desarrollo
```

Separación de capas al estilo *clean architecture*: `domain` no depende de
nada externo (ni de Fiber ni de Postgres); `application` depende solo de
interfaces de dominio; `infrastructure` y `transport` son los únicos que
conocen detalles concretos (Fiber, SQL, Redis).


## Requisitos

- Go 1.25+
- Docker y Docker Compose
- PostgreSQL 16 (via Docker Compose, no requiere instalación local)
- Redis 7 (via Docker Compose, no requiere instalación local)
- Python 3.11+ (solo para el worker de conciliación)

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
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | Conexión a PostgreSQL |
| `DATABASE_URL` | Cadena de conexión completa (derivada de las anteriores) |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB` | Conexión a Redis |
| `JWT_SECRET` | Secreto para firmar/validar tokens JWT |
| `JWT_EXPIRATION_MINUTES` | Duración del token emitido |

## Ejecución

```bash
docker compose up --build
```

Esto levanta la API, PostgreSQL y Redis. La API queda disponible en
`http://localhost:8080`.

_(Instrucciones de ejecución local sin Docker: pendiente, se agregan cuando el
servidor HTTP esté implementado.)_

## Migraciones

Las migraciones son archivos SQL versionados en `migrations/`, con un par
`.up.sql`/`.down.sql` por cada paso (`0001_init_schema.up.sql` crea el
esquema inicial: `merchants`, `payments`, `payment_status_history`; su
`.down.sql` lo revierte). Se aplican con
[`golang-migrate`](https://github.com/golang-migrate/migrate) a través de su
imagen Docker oficial, sin necesidad de instalar nada localmente.

Con Postgres ya levantado (`docker compose up -d postgres`) y tu `.env`
completo:

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

Por ahora cubre `internal/domain` (18 tests): creación válida de
`Merchant`/`Payment`, cada validación individual fallando (nombre vacío,
email inválido, monto ≤ 0, moneda no soportada, método de pago inválido,
referencia externa/llave de idempotencia faltantes), la tabla completa de
transiciones de estado válidas e inválidas, y el caso de no poder volver a
`PENDING` desde un estado terminal. Son pruebas unitarias puras, sin base
de datos ni HTTP de por medio.

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
(`document_number` duplicado, `idempotency_key` duplicada, y
`merchant_id + external_reference` duplicados), actualización de estado, y
creación/listado del historial de estados.

_(Se irán agregando, por fase: pruebas de los endpoints HTTP con
idempotencia y concurrencia, y las del worker de conciliación en Python.)_

## Decisiones técnicas

- **Arquitectura — puertos y adaptadores (hexagonal):** `internal/domain`
  no importa nada externo (ni Fiber, ni SQL, ni Redis); `internal/application`
  define únicamente las interfaces ("puertos") que necesita de la
  infraestructura (`MerchantRepository`, `PaymentRepository`, etc. en
  `ports.go`), sin saber si detrás hay Postgres, otra base, o un mock;
  `internal/infrastructure` (Postgres, Redis) y `internal/transport` (HTTP)
  son los "adaptadores" que implementan esos puertos o los consumen.
  - **Testabilidad:** los casos de uso (Fase 3+) se pueden probar contra un
    repositorio falso en memoria, sin levantar Postgres — las pruebas de
    reglas de negocio no dependen de infraestructura real.
  - **Reemplazabilidad:** cambiar de Postgres a otra base de datos, o
    agregar una capa de cache, solo toca `internal/infrastructure` — el
    dominio y los casos de uso no se enteran ni cambian.
  - **Protección del dominio:** las reglas de negocio quedan aisladas de
    detalles que cambian con el tiempo (versión de Fiber, del driver SQL,
    etc.), en vez de estar mezcladas con código de framework.
- **Framework HTTP:** Fiber, por ser el preferido según el anexo de la
  prueba y su bajo overhead sobre `fasthttp`.
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
- **Autenticación:** JWT.
- **Redis:** usado como componente para (1) idempotencia
  rápida en la creación de pagos junto a la restricción única en Postgres,
  y (2) cache del endpoint de resumen por comercio, invalidada al cambiar
  el estado de un pago.

_(Se irán agregando las decisiones puntuales de cada fase, con su
justificación.)_

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
- _(Se irán agregando más limitaciones conforme surjan en fases
  posteriores.)_