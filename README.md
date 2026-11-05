# ПЗ №14 — Оптимизация запросов к БД. Connection pool

**Студент:** Абдулатифзода Акмал, группа ЭФМО-0126
**Дисциплина:** Технологии индустриального программирования (преподаватель Адышкин С. С.)

Продолжение Notes API из [ПЗ 11](https://github.com/akmalabdulatifzoda-gif/pz11-notes-api)–[ПЗ 12](https://github.com/akmalabdulatifzoda-gif/pz12-swagger):
хранилище в памяти заменено на **PostgreSQL 16** через `pgx/v5` + `pgxpool`, добавлены индексы,
keyset-пагинация, батчинг вместо N+1, кэш Redis, раздельные пулы чтения/записи и метрики пула.

**Стек:** Go 1.24 · PostgreSQL 16 (+ pg_stat_statements) · pgx v5.7 (pgxpool) · go-redis v9 · chi · swag · hey

## Запуск

```bash
make db-up            # PostgreSQL 16 + Redis в Docker (или свой локальный PostgreSQL)
make migrate seed     # схема + 1 000 000 заметок
make index            # индексы оптимизации (make unindex — откатить)
make run-cache        # API на :8080 с кэшем Redis; make run — без кэша
make test             # интеграционные тесты на базе notes_test
make experiment       # полный замер «до/после» + подбор пула (~7 минут)
```

Свой PostgreSQL вместо Docker: `DATABASE_URL=postgres://user@localhost:5432/notes?sslmode=disable make run`.

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `DATABASE_URL`      | `postgres://notes:pass@localhost:5432/notes` | основной сервер (запись) |
| `DATABASE_READ_URL` | = `DATABASE_URL` | реплика для чтения (read/write split) |
| `DB_READ_MAX_CONNS` / `DB_READ_MIN_CONNS` | 10 / 2 | пул чтения |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | 5 / 1 | пул записи |
| `REDIS_ADDR` | — | включает кэш `GET /notes/{id}` (TTL 60 с) |
| `API_TOKEN`  | — | Bearer-токен для POST/PATCH/DELETE |

## Что оптимизировано

| Проблема «до» | Решение «после» |
|---|---|
| `OFFSET 500000` — Seq Scan + внешняя сортировка на диск (414 мс) | keyset `WHERE (created_at, id) < ($1,$2)` + индекс `(created_at, id)` (0.1 мс) |
| поиск — обратный проход по PK с `to_tsvector` на каждой строке (1.4 с) | GIN-индекс + `CTE MATERIALIZED`, чтобы планировщик использовал индекс (0.09 мс) |
| N+1: 20 запросов `WHERE id = $1` на один HTTP-запрос | один `WHERE id = ANY($1)` |
| каждое чтение по id идёт в БД | cache-aside в Redis, инвалидация при PATCH/DELETE |
| `X-Total-Count` из ПЗ 12 требовал бы `count(*)` по 1 млн строк | убран, вместо него курсор `X-Next-Cursor` |

## Результаты (2 vCPU, БД и API на одной машине, 20 клиентов, 15 с)

| Сценарий | до: RPS / p95 / ошибки | после: RPS / p95 / ошибки |
|---|---|---|
| список, страница 500 000 | 0 успешных, 100% таймаутов (2 с) | 6 072 / 7.0 мс / 0% |
| поиск `424242` | 0 успешных, 100% таймаутов | 7 710 / 5.4 мс / 0% |
| 20 заметок по id | 2 137 / 21.9 мс / 0% | 9 245 / 4.8 мс / 0% |
| `GET /notes/{id}` | 12 417 / 3.7 мс / 0% | 15 969 / 2.9 мс / 0% (Redis) |

Пул чтения: оптимум **10** соединений (макс. RPS и минимальный p99) — см. `results/pool_sweep.csv`.
Все сырые выводы hey, планы EXPLAIN и снимки pg_stat_statements — в `results/`. Отчёт — в `report/`.
