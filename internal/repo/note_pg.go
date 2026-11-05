package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
)

// NoteRepoPG — репозиторий заметок в PostgreSQL.
// Запись идёт через пул w, чтение — через пул r (read/write split).
// Если реплики нет, оба пула смотрят на один сервер, но лимиты у них свои:
// тяжёлое чтение не может «съесть» все соединения, нужные для записи.
type NoteRepoPG struct {
	w *pgxpool.Pool
	r *pgxpool.Pool
}

func NewNoteRepoPG(write, read *pgxpool.Pool) *NoteRepoPG {
	if read == nil {
		read = write
	}
	return &NoteRepoPG{w: write, r: read}
}

const noteCols = `id, title, content, created_at, updated_at`

func scanNote(row pgx.Row) (core.Note, error) {
	var n core.Note
	err := row.Scan(&n.ID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Note{}, core.ErrNotFound
	}
	return n, err
}

func collectNotes(rows pgx.Rows, capHint int) ([]core.Note, error) {
	defer rows.Close()
	out := make([]core.Note, 0, capHint) // заранее выделяем слайс под страницу
	for rows.Next() {
		var n core.Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *NoteRepoPG) Create(ctx context.Context, n core.Note) (core.Note, error) {
	// pgx кэширует этот запрос как prepared statement на каждом соединении.
	row := r.w.QueryRow(ctx,
		`INSERT INTO notes (title, content) VALUES ($1, $2) RETURNING `+noteCols,
		n.Title, n.Content)
	return scanNote(row)
}

func (r *NoteRepoPG) GetByID(ctx context.Context, id int64) (core.Note, error) {
	return scanNote(r.r.QueryRow(ctx, `SELECT `+noteCols+` FROM notes WHERE id = $1`, id))
}

// ListOffset — «до»: OFFSET заставляет БД прочитать и отбросить offset строк.
func (r *NoteRepoPG) ListOffset(ctx context.Context, limit, offset int) ([]core.Note, error) {
	rows, err := r.r.Query(ctx, `
		SELECT `+noteCols+`
		FROM notes
		ORDER BY created_at DESC, id DESC
		OFFSET $1 LIMIT $2`, offset, limit)
	if err != nil {
		return nil, err
	}
	return collectNotes(rows, limit)
}

// ListKeyset — «после»: продолжаем с последней увиденной пары (created_at, id)
// по индексу idx_notes_created_id — стоимость не зависит от глубины страницы.
func (r *NoteRepoPG) ListKeyset(ctx context.Context, limit int, after *core.Cursor) ([]core.Note, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if after == nil {
		rows, err = r.r.Query(ctx, `
			SELECT `+noteCols+`
			FROM notes
			ORDER BY created_at DESC, id DESC
			LIMIT $1`, limit)
	} else {
		rows, err = r.r.Query(ctx, `
			SELECT `+noteCols+`
			FROM notes
			WHERE (created_at, id) < ($1, $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $3`, after.CreatedAt, after.ID, limit)
	}
	if err != nil {
		return nil, err
	}
	return collectNotes(rows, limit)
}

// GetManyN1 — «до»: N отдельных запросов (N round-trip до БД и N захватов соединения).
func (r *NoteRepoPG) GetManyN1(ctx context.Context, ids []int64) ([]core.NoteShort, error) {
	out := make([]core.NoteShort, 0, len(ids))
	for _, id := range ids {
		var s core.NoteShort
		err := r.r.QueryRow(ctx, `SELECT id, title FROM notes WHERE id = $1`, id).Scan(&s.ID, &s.Title)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// GetMany — «после»: один запрос с массивом id.
func (r *NoteRepoPG) GetMany(ctx context.Context, ids []int64) ([]core.NoteShort, error) {
	rows, err := r.r.Query(ctx, `SELECT id, title FROM notes WHERE id = ANY($1) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return collectShort(rows, len(ids))
}

func collectShort(rows pgx.Rows, capHint int) ([]core.NoteShort, error) {
	defer rows.Close()
	out := make([]core.NoteShort, 0, capHint)
	for rows.Next() {
		var s core.NoteShort
		if err := rows.Scan(&s.ID, &s.Title); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Search — полнотекстовый поиск по заголовку.
//
// Исходный запрос (WHERE ... @@ ... ORDER BY id DESC LIMIT n) планировщик выполнял
// обратным проходом по первичному ключу с проверкой to_tsvector для каждой строки:
// для редкого слова или слова без совпадений это чтение всей таблицы (~1.4 с на 1 млн строк),
// даже при наличии GIN-индекса — статистика завышает число совпадений.
// CTE MATERIALIZED заставляет сначала найти совпадения через GIN (Bitmap Index Scan),
// и только потом отсортировать их. Худший случай теперь ограничен числом совпадений.
func (r *NoteRepoPG) Search(ctx context.Context, q string, limit int, naive bool) ([]core.NoteShort, error) {
	if naive { // исходная версия запроса — для сравнения «до/после»
		rows, err := r.r.Query(ctx, `
		SELECT id, title
		FROM notes
		WHERE to_tsvector('simple', title) @@ plainto_tsquery('simple', $1)
		ORDER BY id DESC
		LIMIT $2`, q, limit)
		if err != nil {
			return nil, err
		}
		return collectShort(rows, limit)
	}
	rows, err := r.r.Query(ctx, `
		WITH m AS MATERIALIZED (
			SELECT id, title
			FROM notes
			WHERE to_tsvector('simple', title) @@ plainto_tsquery('simple', $1)
		)
		SELECT id, title FROM m
		ORDER BY id DESC
		LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	return collectShort(rows, limit)
}

// Update — частичное обновление в транзакции READ COMMITTED:
// SELECT ... FOR UPDATE блокирует строку, затем UPDATE; один COMMIT.
func (r *NoteRepoPG) Update(ctx context.Context, id int64, apply func(*core.Note) error) (core.Note, error) {
	tx, err := r.w.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return core.Note{}, err
	}
	defer tx.Rollback(ctx) // no-op после Commit

	n, err := scanNote(tx.QueryRow(ctx, `SELECT `+noteCols+` FROM notes WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return core.Note{}, err
	}
	if err := apply(&n); err != nil {
		return core.Note{}, err
	}
	now := time.Now().UTC()
	n.UpdatedAt = &now
	if _, err := tx.Exec(ctx,
		`UPDATE notes SET title = $1, content = $2, updated_at = $3 WHERE id = $4`,
		n.Title, n.Content, n.UpdatedAt, n.ID); err != nil {
		return core.Note{}, err
	}
	return n, tx.Commit(ctx)
}

func (r *NoteRepoPG) Delete(ctx context.Context, id int64) error {
	tag, err := r.w.Exec(ctx, `DELETE FROM notes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}
