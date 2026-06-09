#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Starting service"

: "${DATABASE_URL:?DATABASE_URL is required. Set it in Render Environment variables to your Postgres connection string.}"

echo "[run.sh] Running DB migrations"
goose -dir ./db/migrations postgres "${DATABASE_URL}" up

echo "[run.sh] Starting Caddy"
caddy run --config /etc/caddy/Caddyfile &

echo "[run.sh] Starting Go app"
exec /app/bin/app
