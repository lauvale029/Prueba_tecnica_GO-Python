#!/usr/bin/env bash
set -euo pipefail

# Genera el código Go a partir de sqlc.yaml y las queries en
# internal/infrastructure/postgres/queries, usando la imagen Docker
# oficial de sqlc para no tener que instalar el binario localmente.
#
# Uso:
#   scripts/sqlc-generate.sh

MSYS_NO_PATHCONV=1 docker run --rm \
  -v "$(pwd):/src" \
  -w /src \
  sqlc/sqlc \
  generate