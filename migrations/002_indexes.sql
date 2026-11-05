-- Оптимизация (состояние «после»).

-- Полнотекстовый поиск по заголовку: GIN по выражению.
-- Выражение должно совпадать с тем, что стоит в WHERE запроса.
CREATE INDEX IF NOT EXISTS idx_notes_title_gin
    ON notes USING GIN (to_tsvector('simple', title));

-- Keyset-пагинация: ORDER BY created_at DESC, id DESC
-- B-Tree читается в обратном направлении, отдельный DESC-индекс не нужен.
CREATE INDEX IF NOT EXISTS idx_notes_created_id
    ON notes (created_at, id);

ANALYZE notes;
