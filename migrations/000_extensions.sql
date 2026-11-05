-- Выполняется суперпользователем (в docker-compose — автоматически при инициализации).
-- Требует shared_preload_libraries = 'pg_stat_statements' в postgresql.conf.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
