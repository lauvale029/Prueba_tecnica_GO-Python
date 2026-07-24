#!/usr/bin/env bash
set -euo pipefail

# Aplica las migraciones SQL contra el contenedor de Postgres levantado
# por docker-compose, usando la imagen oficial de golang-migrate para no
# tener que instalar el CLI localmente.
#
# Requiere que el servicio "postgres" ya esté corriendo:
#   docker compose up -d postgres
#
# Uso:
#   scripts/migrate.sh up
#   scripts/migrate.sh down 1
#   scripts/migrate.sh version

set -a
[ -f .env ] && source .env
set +a

: "${DB_USER:?DB_USER is not set. Copy .env.example to .env and fill it in.}"
: "${DB_PASSWORD:?DB_PASSWORD is not set}"
: "${DB_NAME:?DB_NAME is not set}"

# El host es el *nombre del servicio* de Postgres en la red "mova" de
# compose, no "localhost": este script corre el CLI de migrate en su
# propio contenedor, conectado a esa misma red Docker, no en la máquina
# anfitriona.
INTERNAL_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/${DB_NAME}?sslmode=disable"

# MSYS_NO_PATHCONV evita que Git Bash en Windows reescriba las rutas
# absolutas del lado del contenedor (/migrations) como si fueran rutas de
# Windows mal formadas. En Linux/macOS no tiene ningún efecto.
MSYS_NO_PATHCONV=1 docker run --rm \
  --network mova \
  -v "$(pwd)/migrations:/migrations" \
  migrate/migrate \
  -path=/migrations \
  -database "$INTERNAL_DATABASE_URL" \
  "$@"