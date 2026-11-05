package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http/handlers"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http/redoc"
)

type Options struct {
	APIToken       string        // пусто — авторизация выключена
	RequestTimeout time.Duration // общий таймаут запроса, включая ожидание соединения из пула
	AccessLog      bool
	PoolStats      func() any // данные для /debug/pool
}

// timeout ограничивает время запроса: если пул исчерпан, Acquire не ждёт вечно.
func timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func NewRouter(h *handlers.Handler, o Options) *chi.Mux {
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 2 * time.Second
	}
	r := chi.NewRouter()
	if o.AccessLog {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	r.Route("/api/v1/notes", func(r chi.Router) {
		r.Use(timeout(o.RequestTimeout))
		r.Get("/", h.ListNotes)
		r.Get("/batch", h.BatchNotes)
		r.Get("/search", h.SearchNotes)
		r.Get("/{id}", h.GetNote)

		r.Group(func(r chi.Router) {
			r.Use(BearerAuth(o.APIToken))
			r.Post("/", h.CreateNote)
			r.Patch("/{id}", h.PatchNote)
			r.Delete("/{id}", h.DeleteNote)
		})
	})

	// Метрики пула: in-use / idle / ожидания.
	r.Get("/debug/pool", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var v any = map[string]string{}
		if o.PoolStats != nil {
			v = o.PoolStats()
		}
		_ = json.NewEncoder(w).Encode(v)
	})

	r.Get("/docs", http.RedirectHandler("/docs/index.html", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.PersistAuthorization(true),
	))
	r.Get("/redoc", redoc.Page)
	r.Get("/redoc/redoc.standalone.js", redoc.Script)
	return r
}
