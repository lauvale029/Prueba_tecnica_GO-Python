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

_(Pendiente — se documenta en la fase de dominio y migraciones: herramienta
usada, cómo aplicarlas y cómo revertirlas.)_

## Pruebas

_(Pendiente — se documenta a medida que se agreguen los paquetes de pruebas
por fase: dominio, repositorios, endpoints, worker de Python.)_

## Decisiones técnicas

- **Framework HTTP:** Fiber, por ser el preferido según el anexo de la
  prueba y su bajo overhead sobre `fasthttp`.
- **Acceso a datos:** `sqlc` + `database/sql` (driver `pgx`). Se prefirió
  sobre un ORM para mantener control explícito del SQL y generar código
  type-safe sin reflection en runtime.
- **Dinero:** columnas `NUMERIC` en PostgreSQL mapeadas a
  `shopspring/decimal` en Go, evitando por completo el uso de `float64`
  para valores monetarios.
- **Autenticación:** JWT.
- **Redis:** usado como componente para (1) idempotencia
  rápida en la creación de pagos junto a la restricción única en Postgres,
  y (2) cache del endpoint de resumen por comercio, invalidada al cambiar
  el estado de un pago.

_(Se irán agregando las decisiones puntuales de cada fase, con su
justificación.)_

## Supuestos

_(Pendiente — se documentan conforme surjan durante la implementación.)_

## Limitaciones

_(Pendiente.)_