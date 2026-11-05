#!/usr/bin/env bash
# Полный эксперимент ПЗ 14: «до» (без индексов, OFFSET, N+1, без кэша)
# и «после» (индексы, keyset, батч, Redis) под одинаковой нагрузкой,
# снимки pg_stat_statements и подбор размера пула.
#
# Требует: PostgreSQL с pg_stat_statements, Redis, hey, данные из scripts/seed.sql.
# Переменные: DATABASE_URL, PGSUPER (строка подключения суперпользователя для reset статистики).
set -euo pipefail
cd "$(dirname "$0")/.."

DB=${DATABASE_URL:-postgres://notes:pass@localhost:5432/notes?sslmode=disable}
PGSUPER=${PGSUPER:-$DB}
export HEY=${HEY:-$(go env GOPATH)/bin/hey}
B=http://localhost:8080/api/v1/notes
IDS="11,22,33,44,55,66,77,88,99,111,222,333,444,555,666,777,888,999,4242,424242"
DUR=${DUR:-15}

mkdir -p results/load
echo "name,clients,rps,avg_ms,p50_ms,p95_ms,p99_ms,error_pct,requests" > results/summary.csv
go build -o bin/api ./cmd/api

start() { pkill -f bin/api 2>/dev/null || true; sleep 0.5; env "$@" ./bin/api > results/server.log 2>&1 & sleep 1.5; }
waits() { curl -s localhost:8080/debug/pool | python3 -c 'import json,sys; print(json.load(sys.stdin)["read"]["emptyAcquireCount"])'; }
pgss_reset() { psql -q "$PGSUPER" -c "SELECT pg_stat_statements_reset()" > /dev/null; }
pgss_dump() {
  psql "$DB" -c "SELECT left(regexp_replace(query, '\s+', ' ', 'g'), 95) AS query, calls,
         round(total_exec_time::numeric, 0) AS total_ms, round(mean_exec_time::numeric, 2) AS mean_ms,
         round(max_exec_time::numeric, 1) AS max_ms
       FROM pg_stat_statements WHERE query ILIKE '%from notes%'
       ORDER BY total_exec_time DESC LIMIT 8" > "results/pgss_$1.txt"
  psql "$DB" -At -F'|' -c "SELECT regexp_replace(query, '\s+', ' ', 'g'), calls, total_exec_time, mean_exec_time
       FROM pg_stat_statements WHERE query ILIKE '%from notes%' ORDER BY total_exec_time DESC" > "results/pgss_$1.csv"
}

echo "### ДО: без индексов, OFFSET, N+1, без кэша"
psql -q "$DB" -f migrations/002_indexes.down.sql 2>/dev/null
start
pgss_reset
./scripts/load.sh before_list_offset     "$B?offset=500000&limit=20" 20 "$DUR"
./scripts/load.sh before_list_offset_c1  "$B?offset=500000&limit=20" 1  "$DUR"
./scripts/load.sh before_search          "$B/search?q=424242&mode=naive" 20 "$DUR"
./scripts/load.sh before_search_c1       "$B/search?q=424242&mode=naive"        1  "$DUR"
./scripts/load.sh before_batch_n1        "$B/batch?mode=n1&ids=$IDS" 20 "$DUR"
./scripts/load.sh before_get_nocache     "$B/424242"                 20 "$DUR"
pgss_dump before

echo "### ПОСЛЕ: индексы, keyset, батч, Redis"
psql -q "$DB" -f migrations/002_indexes.sql
CUR=$(psql "$DB" -At -c "SELECT created_at, id FROM notes ORDER BY created_at DESC, id DESC OFFSET 499999 LIMIT 1" \
  | awk -F'|' '{print $1"|"$2}')
# Курсор строим через API: первая страница -> идём по X-Next-Cursor нельзя на 25 000 страниц,
# поэтому кодируем тот же формат, что и сервис: base64url("<unix-nano>:<id>").
CUR=$(go run ./scripts/cursor "$CUR")
redis-cli FLUSHALL > /dev/null
start REDIS_ADDR=localhost:6379
pgss_reset
./scripts/load.sh after_list_keyset         "$B?limit=20&cursor=$CUR"   20 "$DUR"
./scripts/load.sh after_list_offset_indexed "$B?offset=500000&limit=20" 20 "$DUR"
./scripts/load.sh after_search              "$B/search?q=424242"        20 "$DUR"
./scripts/load.sh after_batch               "$B/batch?ids=$IDS"         20 "$DUR"
./scripts/load.sh after_get_cache           "$B/424242"                 20 "$DUR"
pgss_dump after
start  # без кэша — чтобы отделить эффект Redis от остального
./scripts/load.sh after_get_nocache         "$B/424242"                 20 "$DUR"

echo "### Подбор размера пула чтения (DB_READ_MAX_CONNS)"
echo "pool,scenario,clients,rps,avg_ms,p50_ms,p95_ms,p99_ms,error_pct,requests,acquire_waits" > results/pool_sweep.csv
for n in 1 2 5 10 20 50 100; do
  start DB_READ_MAX_CONNS=$n DB_READ_MIN_CONNS=1
  for sc in batch offset; do
    if [ $sc = batch ]; then url="$B/batch?ids=$IDS"; c=100; else url="$B?offset=100000&limit=20"; c=50; fi
    w0=$(waits)
    line=$(./scripts/load.sh "pool${n}_$sc" "$url" $c 10 | tail -1)
    # emptyAcquireCount — сколько раз запрос ждал свободное соединение (счётчик накопительный, берём разность)
    echo "$n,$sc,$(echo "$line" | cut -d, -f2-),$(( $(waits) - w0 ))" >> results/pool_sweep.csv
  done
done
pkill -f bin/api || true
echo "Готово: results/summary.csv, results/pool_sweep.csv, results/pgss_*.txt"
