#!/usr/bin/env bash
# Exercise only containers, networks, and databases created by this invocation.
set -euo pipefail

image="${1:-sokomoko:local}"
scope="sokomoko-smoke-${RANDOM}-$$"
database_container="${scope}-db"
app_container="${scope}-app"
backup_file="$(mktemp)"

cleanup() {
  docker rm -f "$app_container" "$database_container" >/dev/null 2>&1 || true
  docker network rm "$scope" >/dev/null 2>&1 || true
  rm -f "$backup_file"
}
trap cleanup EXIT

docker network create "$scope" >/dev/null
docker run -d --name "$database_container" --network "$scope" --network-alias db \
  -e POSTGRES_USER=sokomoko -e POSTGRES_PASSWORD=smoke-only-password \
  -e POSTGRES_DB=sokomoko postgres:17-alpine >/dev/null

database_ready=false
for ((attempt = 0; attempt < 60; attempt++)); do
  if docker exec "$database_container" pg_isready -h 127.0.0.1 -U sokomoko -d sokomoko >/dev/null 2>&1; then
    database_ready=true
    break
  fi
  sleep 1
done
if [[ "$database_ready" != true ]]; then
  docker logs "$database_container"
  exit 1
fi

owner_url='postgres://sokomoko:smoke-only-password@db:5432/sokomoko?sslmode=disable'
runtime_url='postgres://sokomoko_app:smoke-only-password@db:5432/sokomoko?sslmode=disable'
docker run --rm --network "$scope" -e ENV=production -e DATABASE_URL="$owner_url" "$image" migrate
docker run --rm --network "$scope" -e ENV=production -e DATABASE_URL="$owner_url" "$image" migrate

docker exec -i "$database_container" psql -U sokomoko -d sokomoko -v ON_ERROR_STOP=1 <<'SQL'
CREATE ROLE sokomoko_app LOGIN PASSWORD 'smoke-only-password';
GRANT USAGE ON SCHEMA public TO sokomoko_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO sokomoko_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO sokomoko_app;
REVOKE INSERT, UPDATE, DELETE ON schema_migrations FROM sokomoko_app;
SQL

docker run -d --name "$app_container" --network "$scope" --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true \
  -e ENV=production -e DATABASE_URL="$runtime_url" \
  -e ALLOWED_HOSTS=shop.example.com,admin.example.com,partner.example.com \
  -e PASSWORD_RESET_BASE_URL=https://shop.example.com \
  -e SMTP_HOST=smtp.example.com -e SMTP_FROM=support@example.com \
  "$image" serve >/dev/null

app_ready=false
for ((attempt = 0; attempt < 30; attempt++)); do
  if docker exec "$app_container" /usr/local/bin/sokomoko healthcheck >/dev/null 2>&1; then
    app_ready=true
    break
  fi
  sleep 1
done
if [[ "$app_ready" != true ]]; then
  docker logs "$app_container"
  exit 1
fi
for host in shop.example.com admin.example.com partner.example.com; do
  docker exec -e ALLOWED_HOSTS="$host" "$app_container" /usr/local/bin/sokomoko healthcheck
done

privileges="$(docker exec "$database_container" psql -U sokomoko -d sokomoko -Atc \
  "SELECT has_schema_privilege('sokomoko_app', 'public', 'CREATE'), has_table_privilege('sokomoko_app', 'schema_migrations', 'INSERT')")"
[[ "$privileges" == 'f|f' ]]
products="$(docker exec "$database_container" psql -U sokomoko -d sokomoko -Atc 'SELECT count(*) FROM products')"
[[ "$products" == '0' ]]

# Verify SIGTERM exits cleanly before the database dependency disappears.
docker stop --time 40 "$app_container" >/dev/null
[[ "$(docker inspect --format '{{.State.ExitCode}}' "$app_container")" == '0' ]]

# Restore to another database; never restore over the source database.
docker exec "$database_container" pg_dump -U sokomoko -d sokomoko --format=custom --no-owner --no-acl > "$backup_file"
docker exec "$database_container" createdb -U sokomoko sokomoko_restore
docker exec -i "$database_container" pg_restore -U sokomoko -d sokomoko_restore \
  --exit-on-error --single-transaction --no-owner --no-acl < "$backup_file"
snapshot_sql='SELECT (SELECT count(*) FROM schema_migrations), (SELECT count(*) FROM warehouses)'
source_snapshot="$(docker exec "$database_container" psql -U sokomoko -d sokomoko -Atc "$snapshot_sql")"
restored_snapshot="$(docker exec "$database_container" psql -U sokomoko -d sokomoko_restore -Atc "$snapshot_sql")"
[[ "$source_snapshot" == "$restored_snapshot" ]]

printf '%s\n' 'Image smoke check passed: release migration, restricted runtime, three hosts, clean shutdown, isolated restore.'
