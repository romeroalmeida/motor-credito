#!/bin/sh
# Cria o usuário de login da aplicação (motor_api) como membro de motor_app.
# motor_app recebe as permissões na migration 0001; aqui só nasce o papel, se ainda não existir.
set -eu
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v senha="$POSTGRES_APP_PASSWORD" <<'SQL'
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'motor_app') THEN
        CREATE ROLE motor_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END $$;
CREATE ROLE motor_api LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD :'senha' IN ROLE motor_app;
SQL
