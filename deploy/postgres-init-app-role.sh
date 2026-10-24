#!/bin/sh
set -eu

psql --set=ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set=app_password="$POSTGRES_APP_PASSWORD" <<'SQL'
CREATE ROLE sokomoko_app LOGIN PASSWORD :'app_password';
GRANT CONNECT ON DATABASE sokomoko TO sokomoko_app;
GRANT USAGE ON SCHEMA public TO sokomoko_app;
ALTER DEFAULT PRIVILEGES FOR ROLE sokomoko_migrate IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sokomoko_app;
ALTER DEFAULT PRIVILEGES FOR ROLE sokomoko_migrate IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO sokomoko_app;
SQL
