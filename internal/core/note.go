package core

import (
	"errors"
	"time"
)

// Note — сущность «Заметка».
type Note struct {
	ID        int64      `json:"id" example:"1"`
	Title     string     `json:"title" example:"Идея для лекции"`
	Content   string     `json:"content" example:"Добавить пример структуры API"`
	CreatedAt time.Time  `json:"createdAt" example:"2026-11-05T12:00:00Z"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty" example:"2026-11-05T12:30:00Z"`
}

// NoteShort — облегчённая проекция для батч-запросов и поиска (без content).
type NoteShort struct {
	ID    int64  `json:"id" example:"1"`
	Title string `json:"title" example:"Redis cache #42"`
}

// NoteCreate — DTO для создания заметки.
type NoteCreate struct {
	Title   string `json:"title" example:"Новая заметка" minLength:"1" maxLength:"200"`
	Content string `json:"content" example:"Текст заметки"`
}

// NoteUpdate — DTO для частичного обновления: nil означает «поле не меняется».
type NoteUpdate struct {
	Title   *string `json:"title,omitempty" example:"Обновлено" maxLength:"200"`
	Content *string `json:"content,omitempty" example:"Новый текст"`
}

// ErrorResponse — единый формат ошибки API.
type ErrorResponse struct {
	Error string `json:"error" example:"описание ошибки"`
}

// Cursor — позиция для keyset-пагинации: последняя увиденная пара (created_at, id).
type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

// Page — страница списка. NextCursor пустой, если дальше данных нет.
type Page struct {
	Items      []Note
	NextCursor string
}

var (
	ErrNotFound   = errors.New("note not found")
	ErrValidation = errors.New("validation error")
)
