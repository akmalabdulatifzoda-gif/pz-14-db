package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
)

// NoteRepository — слой хранения (реализация: repo.NoteRepoPG).
type NoteRepository interface {
	Create(ctx context.Context, n core.Note) (core.Note, error)
	GetByID(ctx context.Context, id int64) (core.Note, error)
	ListOffset(ctx context.Context, limit, offset int) ([]core.Note, error)
	ListKeyset(ctx context.Context, limit int, after *core.Cursor) ([]core.Note, error)
	GetManyN1(ctx context.Context, ids []int64) ([]core.NoteShort, error)
	GetMany(ctx context.Context, ids []int64) ([]core.NoteShort, error)
	Search(ctx context.Context, q string, limit int, naive bool) ([]core.NoteShort, error)
	Update(ctx context.Context, id int64, apply func(*core.Note) error) (core.Note, error)
	Delete(ctx context.Context, id int64) error
}

// Cache — необязательный кэш заметок по id (repo в Redis).
type Cache interface {
	Get(ctx context.Context, id int64) (core.Note, bool)
	Set(ctx context.Context, n core.Note)
	Invalidate(ctx context.Context, id int64)
}

type NoteService struct {
	repo  NoteRepository
	cache Cache // nil — кэш выключен
}

func NewNoteService(r NoteRepository, c Cache) *NoteService {
	return &NoteService{repo: r, cache: c}
}

const (
	DefaultLimit = 20
	MaxLimit     = 100
	MaxBatch     = 100
)

func clampLimit(l int) int {
	if l <= 0 {
		return DefaultLimit
	}
	if l > MaxLimit {
		return MaxLimit
	}
	return l
}

func validateTitle(t string) error {
	if strings.TrimSpace(t) == "" {
		return fmt.Errorf("%w: title is required", core.ErrValidation)
	}
	if len([]rune(t)) > 200 {
		return fmt.Errorf("%w: title must be at most 200 characters", core.ErrValidation)
	}
	return nil
}

func (s *NoteService) Create(ctx context.Context, in core.NoteCreate) (core.Note, error) {
	if err := validateTitle(in.Title); err != nil {
		return core.Note{}, err
	}
	return s.repo.Create(ctx, core.Note{Title: strings.TrimSpace(in.Title), Content: in.Content})
}

// Get — cache-aside: сначала Redis, при промахе — БД и запись в кэш.
func (s *NoteService) Get(ctx context.Context, id int64) (core.Note, error) {
	if s.cache != nil {
		if n, ok := s.cache.Get(ctx, id); ok {
			return n, nil
		}
	}
	n, err := s.repo.GetByID(ctx, id)
	if err == nil && s.cache != nil {
		s.cache.Set(ctx, n)
	}
	return n, err
}

// ListOffset — старая пагинация (оставлена для сравнения «до/после»).
func (s *NoteService) ListOffset(ctx context.Context, limit, offset int) ([]core.Note, error) {
	if offset < 0 {
		return nil, fmt.Errorf("%w: offset must be >= 0", core.ErrValidation)
	}
	return s.repo.ListOffset(ctx, clampLimit(limit), offset)
}

// List — keyset-пагинация. cursor — непрозрачная строка из X-Next-Cursor.
func (s *NoteService) List(ctx context.Context, limit int, cursor string) (core.Page, error) {
	limit = clampLimit(limit)
	var after *core.Cursor
	if cursor != "" {
		c, err := DecodeCursor(cursor)
		if err != nil {
			return core.Page{}, err
		}
		after = &c
	}
	items, err := s.repo.ListKeyset(ctx, limit, after)
	if err != nil {
		return core.Page{}, err
	}
	p := core.Page{Items: items}
	if len(items) == limit {
		last := items[len(items)-1]
		p.NextCursor = EncodeCursor(core.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return p, nil
}

// GetMany — пакетное получение. n1=true — старый вариант с N запросами.
func (s *NoteService) GetMany(ctx context.Context, ids []int64, n1 bool) ([]core.NoteShort, error) {
	if len(ids) == 0 || len(ids) > MaxBatch {
		return nil, fmt.Errorf("%w: ids must contain 1..%d values", core.ErrValidation, MaxBatch)
	}
	if n1 {
		return s.repo.GetManyN1(ctx, ids)
	}
	return s.repo.GetMany(ctx, ids)
}

// Search — полнотекстовый поиск. naive=true — исходная версия запроса (для сравнения).
func (s *NoteService) Search(ctx context.Context, q string, limit int, naive bool) ([]core.NoteShort, error) {
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("%w: q is required", core.ErrValidation)
	}
	return s.repo.Search(ctx, q, clampLimit(limit), naive)
}

func (s *NoteService) Update(ctx context.Context, id int64, p core.NoteUpdate) (core.Note, error) {
	if p.Title == nil && p.Content == nil {
		return core.Note{}, fmt.Errorf("%w: nothing to update", core.ErrValidation)
	}
	if p.Title != nil {
		if err := validateTitle(*p.Title); err != nil {
			return core.Note{}, err
		}
	}
	n, err := s.repo.Update(ctx, id, func(n *core.Note) error {
		if p.Title != nil {
			n.Title = strings.TrimSpace(*p.Title)
		}
		if p.Content != nil {
			n.Content = *p.Content
		}
		return nil
	})
	if err == nil && s.cache != nil {
		s.cache.Invalidate(ctx, id)
	}
	return n, err
}

func (s *NoteService) Delete(ctx context.Context, id int64) error {
	err := s.repo.Delete(ctx, id)
	if err == nil && s.cache != nil {
		s.cache.Invalidate(ctx, id)
	}
	return err
}

// EncodeCursor: base64url("<unix-nano>:<id>").
func EncodeCursor(c core.Cursor) string {
	raw := strconv.FormatInt(c.CreatedAt.UnixNano(), 10) + ":" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(s string) (core.Cursor, error) {
	bad := fmt.Errorf("%w: invalid cursor", core.ErrValidation)
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return core.Cursor{}, bad
	}
	ts, id, ok := strings.Cut(string(b), ":")
	if !ok {
		return core.Cursor{}, bad
	}
	ns, err1 := strconv.ParseInt(ts, 10, 64)
	n, err2 := strconv.ParseInt(id, 10, 64)
	if err1 != nil || err2 != nil {
		return core.Cursor{}, bad
	}
	return core.Cursor{CreatedAt: time.Unix(0, ns).UTC(), ID: n}, nil
}
