-- Планы ключевых запросов: psql "$DATABASE_URL" -f scripts/explain.sql
\echo '=== OFFSET-пагинация, страница на глубине 500 000'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, title, content, created_at, updated_at FROM notes
ORDER BY created_at DESC, id DESC OFFSET 500000 LIMIT 20;

\echo '=== Keyset-пагинация с той же позиции'
SELECT created_at AS c, id AS i FROM notes ORDER BY created_at DESC, id DESC OFFSET 499999 LIMIT 1 \gset
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, title, content, created_at, updated_at FROM notes
WHERE (created_at, id) < (:'c', :i)
ORDER BY created_at DESC, id DESC LIMIT 20;

\echo '=== Поиск: исходный запрос'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, title FROM notes
WHERE to_tsvector('simple', title) @@ plainto_tsquery('simple', '424242')
ORDER BY id DESC LIMIT 20;

\echo '=== Поиск: переписанный запрос (CTE MATERIALIZED)'
EXPLAIN (ANALYZE, BUFFERS)
WITH m AS MATERIALIZED (
  SELECT id, title FROM notes
  WHERE to_tsvector('simple', title) @@ plainto_tsquery('simple', '424242'))
SELECT id, title FROM m ORDER BY id DESC LIMIT 20;

\echo '=== Батч вместо N+1'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, title FROM notes WHERE id = ANY('{11,22,33,44,55,66,77,88,99,111,222,333,444,555,666,777,888,999,4242,424242}'::bigint[]) ORDER BY id;
