-- Тестовые данные: :rows заметок (по умолчанию 1 000 000).
-- psql -v rows=1000000 -f scripts/seed.sql
\if :{?rows}
\else
  \set rows 1000000
\endif
TRUNCATE notes RESTART IDENTITY;

WITH words AS (
    SELECT ARRAY['go','postgres','redis','docker','api','index','pool','cache',
                 'lecture','idea','todo','bug','release','meeting','plan','review',
                 'kafka','grpc','jwt','swagger','pprof','sql','backup','deploy'] AS w
)
INSERT INTO notes (title, content, created_at)
SELECT
    initcap(w[1 + (g * 7  % 24)]) || ' ' || w[1 + (g * 13 % 23)] || ' #' || g,
    repeat('Текст заметки номер ' || g || '. ', 1 + g % 5),
    now() - make_interval(secs => (1000000 - g) * 60) -- одна заметка в минуту, ~2 года
FROM generate_series(1, :rows) AS g, words;

ANALYZE notes;
SELECT count(*) AS rows, pg_size_pretty(pg_total_relation_size('notes')) AS size FROM notes;
