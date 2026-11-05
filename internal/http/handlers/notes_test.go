package handlers_test

// Интеграционные тесты на настоящем PostgreSQL.
// Запуск: TEST_DATABASE_URL=postgres://notes:pass@localhost:5432/notes_test?sslmode=disable go test ./...
// Без TEST_DATABASE_URL тесты пропускаются.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core/service"
	httpx "github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http/handlers"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/repo"

	_ "github.com/akmalabdulatifzoda-gif/pz-14-db/docs"
)

func newServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()
	pool, err := repo.NewPool(ctx, url, repo.PoolConfigFromEnv("TEST_", 5, 1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	schema, _ := os.ReadFile("../../../migrations/001_schema.sql")
	idx, _ := os.ReadFile("../../../migrations/002_indexes.sql")
	for _, sql := range []string{string(schema), string(idx), "TRUNCATE notes RESTART IDENTITY"} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	h := &handlers.Handler{Svc: service.NewNoteService(repo.NewNoteRepoPG(pool, pool), nil)}
	srv := httptest.NewServer(httpx.NewRouter(h, httpx.Options{APIToken: token}))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestCRUD(t *testing.T) {
	srv := newServer(t, "")
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/api/v1/notes", `{"title":"Первая","content":"тест"}`, 201},
		{"POST", "/api/v1/notes", `{"title":""}`, 400},
		{"GET", "/api/v1/notes/1", "", 200},
		{"GET", "/api/v1/notes/999", "", 404},
		{"PATCH", "/api/v1/notes/1", `{"content":"обновлено"}`, 200},
		{"PATCH", "/api/v1/notes/999", `{"content":"x"}`, 404},
		{"DELETE", "/api/v1/notes/1", "", 204},
		{"DELETE", "/api/v1/notes/1", "", 404},
	}
	for _, c := range cases {
		resp := do(t, srv, c.method, c.path, c.body)
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s %s: got %d, want %d", c.method, c.path, resp.StatusCode, c.want)
		}
	}
}

func TestKeysetMatchesOffset(t *testing.T) {
	srv := newServer(t, "")
	for i := 0; i < 25; i++ {
		do(t, srv, "POST", "/api/v1/notes", `{"title":"Redis note"}`).Body.Close()
	}
	var all []core.Note
	cursor := ""
	for {
		resp := do(t, srv, "GET", "/api/v1/notes?limit=10&cursor="+cursor, "")
		var page []core.Note
		_ = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		all = append(all, page...)
		cursor = resp.Header.Get("X-Next-Cursor")
		if cursor == "" {
			break
		}
	}
	resp := do(t, srv, "GET", "/api/v1/notes?limit=100&offset=0", "")
	var byOffset []core.Note
	_ = json.NewDecoder(resp.Body).Decode(&byOffset)
	resp.Body.Close()
	if len(all) != 25 || len(byOffset) != 25 {
		t.Fatalf("keyset=%d offset=%d, want 25", len(all), len(byOffset))
	}
	for i := range all {
		if all[i].ID != byOffset[i].ID {
			t.Fatalf("position %d: keyset id %d != offset id %d", i, all[i].ID, byOffset[i].ID)
		}
	}
}

func TestBatchAndSearch(t *testing.T) {
	srv := newServer(t, "")
	for _, title := range []string{"Redis cache", "Go pool", "redis cluster"} {
		do(t, srv, "POST", "/api/v1/notes", `{"title":"`+title+`"}`).Body.Close()
	}
	for _, mode := range []string{"batch", "n1"} {
		resp := do(t, srv, "GET", "/api/v1/notes/batch?ids=1,3,999&mode="+mode, "")
		var got []core.NoteShort
		_ = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if len(got) != 2 {
			t.Errorf("mode=%s: got %d notes, want 2", mode, len(got))
		}
	}
	resp := do(t, srv, "GET", "/api/v1/notes/search?q=redis", "")
	var found []core.NoteShort
	_ = json.NewDecoder(resp.Body).Decode(&found)
	resp.Body.Close()
	if len(found) != 2 {
		t.Errorf("search: got %d, want 2", len(found))
	}
	if r := do(t, srv, "GET", "/api/v1/notes/batch?ids=abc", ""); r.StatusCode != 400 {
		t.Errorf("bad ids: got %d", r.StatusCode)
	}
}

func TestBearerAuth(t *testing.T) {
	srv := newServer(t, "secret")
	if r := do(t, srv, "POST", "/api/v1/notes", `{"title":"x"}`); r.StatusCode != 401 {
		t.Errorf("without token: got %d, want 401", r.StatusCode)
	}
}
