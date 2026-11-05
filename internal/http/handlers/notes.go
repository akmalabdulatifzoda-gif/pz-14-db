package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core/service"
)

type Handler struct {
	Svc *service.NoteService
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, core.ErrorResponse{Error: msg})
}

// handleErr переводит ошибки в HTTP-коды. Таймаут ожидания соединения из пула
// или запроса к БД (context deadline) — 503, чтобы клиент мог повторить запрос.
func handleErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, core.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeError(w, http.StatusServiceUnavailable, "database timeout")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func parseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func queryInt(r *http.Request, key string) (int, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, errors.New("invalid " + key)
	}
	return n, nil
}

// ListNotes godoc
//
//	@ID				listNotes
//	@Summary		Список заметок (keyset или offset)
//	@Description	По умолчанию — keyset-пагинация: курсор следующей страницы приходит в заголовке X-Next-Cursor.
//	@Description	Параметр offset включает старую OFFSET-пагинацию (оставлена для сравнения, медленная на больших смещениях).
//	@Tags			notes
//	@Produce		json
//	@Param			limit	query		int		false	"Размер страницы (1–100)"	minimum(1)	maximum(100)	default(20)
//	@Param			cursor	query		string	false	"Курсор из X-Next-Cursor предыдущей страницы"
//	@Param			offset	query		int		false	"Смещение (устаревший режим)"	minimum(0)
//	@Success		200		{array}		core.Note
//	@Header			200		{string}	X-Next-Cursor	"Курсор следующей страницы (нет — страница последняя)"
//	@Failure		400		{object}	core.ErrorResponse
//	@Failure		503		{object}	core.ErrorResponse
//	@Router			/notes [get]
func (h *Handler) ListNotes(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Has("offset") {
		offset, err := queryInt(r, "offset")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		notes, err := h.Svc.ListOffset(r.Context(), limit, offset)
		if err != nil {
			handleErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, notes)
		return
	}
	page, err := h.Svc.List(r.Context(), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if page.NextCursor != "" {
		w.Header().Set("X-Next-Cursor", page.NextCursor)
	}
	writeJSON(w, http.StatusOK, page.Items)
}

// BatchNotes godoc
//
//	@ID				batchNotes
//	@Summary		Получить несколько заметок по id
//	@Description	Один запрос WHERE id = ANY($1). mode=n1 — старый вариант с отдельным запросом на каждый id (для сравнения).
//	@Tags			notes
//	@Produce		json
//	@Param			ids		query		string	true	"Список id через запятую (до 100)"	example(1,2,3)
//	@Param			mode	query		string	false	"batch (по умолчанию) или n1"		Enums(batch, n1)
//	@Success		200		{array}		core.NoteShort
//	@Failure		400		{object}	core.ErrorResponse
//	@Failure		503		{object}	core.ErrorResponse
//	@Router			/notes/batch [get]
func (h *Handler) BatchNotes(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("ids")
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid ids")
			return
		}
		ids = append(ids, id)
	}
	notes, err := h.Svc.GetMany(r.Context(), ids, r.URL.Query().Get("mode") == "n1")
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

// SearchNotes godoc
//
//	@ID				searchNotes
//	@Summary		Полнотекстовый поиск по заголовку
//	@Description	to_tsvector('simple', title) @@ plainto_tsquery('simple', q) через GIN-индекс idx_notes_title_gin (CTE MATERIALIZED: сначала поиск по индексу, потом сортировка)
//	@Tags			notes
//	@Produce		json
//	@Param			q		query		string	true	"Слова для поиска"														example(redis)
//	@Param			limit	query		int		false	"Сколько результатов (1–100)"											default(20)
//	@Param			mode	query		string	false	"optimized (по умолчанию) или naive — исходный запрос для сравнения"	Enums(optimized, naive)
//	@Success		200		{array}		core.NoteShort
//	@Failure		400		{object}	core.ErrorResponse
//	@Failure		503		{object}	core.ErrorResponse
//	@Router			/notes/search [get]
func (h *Handler) SearchNotes(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	notes, err := h.Svc.Search(r.Context(), r.URL.Query().Get("q"), limit, r.URL.Query().Get("mode") == "naive")
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

// CreateNote godoc
//
//	@ID			createNote
//	@Summary	Создать заметку
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Param		input	body		core.NoteCreate	true	"Данные новой заметки"
//	@Success	201		{object}	core.Note
//	@Header		201		{string}	Location	"URL созданной заметки"
//	@Failure	400		{object}	core.ErrorResponse
//	@Failure	401		{object}	core.ErrorResponse
//	@Security	BearerAuth
//	@Router		/notes [post]
func (h *Handler) CreateNote(w http.ResponseWriter, r *http.Request) {
	var in core.NoteCreate
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	n, err := h.Svc.Create(r.Context(), in)
	if err != nil {
		handleErr(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/notes/"+strconv.FormatInt(n.ID, 10))
	writeJSON(w, http.StatusCreated, n)
}

// GetNote godoc
//
//	@ID				getNote
//	@Summary		Получить заметку
//	@Description	Чтение через кэш Redis (cache-aside, TTL 60 с), если сервер запущен с REDIS_ADDR
//	@Tags			notes
//	@Produce		json
//	@Param			id	path		int	true	"ID заметки"	minimum(1)
//	@Success		200	{object}	core.Note
//	@Failure		400	{object}	core.ErrorResponse
//	@Failure		404	{object}	core.ErrorResponse
//	@Router			/notes/{id} [get]
func (h *Handler) GetNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := h.Svc.Get(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// PatchNote godoc
//
//	@ID				patchNote
//	@Summary		Обновить заметку (частично)
//	@Description	Транзакция READ COMMITTED: SELECT ... FOR UPDATE + UPDATE; кэш заметки инвалидируется
//	@Tags			notes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int				true	"ID заметки"	minimum(1)
//	@Param			input	body		core.NoteUpdate	true	"Поля для обновления"
//	@Success		200		{object}	core.Note
//	@Failure		400		{object}	core.ErrorResponse
//	@Failure		401		{object}	core.ErrorResponse
//	@Failure		404		{object}	core.ErrorResponse
//	@Security		BearerAuth
//	@Router			/notes/{id} [patch]
func (h *Handler) PatchNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var p core.NoteUpdate
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	n, err := h.Svc.Update(r.Context(), id, p)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// DeleteNote godoc
//
//	@ID				deleteNote
//	@Summary		Удалить заметку
//	@Description	Кэш заметки инвалидируется
//	@Tags			notes
//	@Param			id	path	int	true	"ID заметки"	minimum(1)
//	@Success		204	"No Content"
//	@Failure		400	{object}	core.ErrorResponse
//	@Failure		401	{object}	core.ErrorResponse
//	@Failure		404	{object}	core.ErrorResponse
//	@Security		BearerAuth
//	@Router			/notes/{id} [delete]
func (h *Handler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Svc.Delete(r.Context(), id); err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
