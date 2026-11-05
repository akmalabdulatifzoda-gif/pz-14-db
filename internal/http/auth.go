package httpx

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// BearerAuth — упрощённая проверка заголовка Authorization: Bearer <token>.
// Если token пустой (переменная API_TOKEN не задана), проверка отключена.
// В следующих ПЗ статический токен будет заменён на JWT.
func BearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("WWW-Authenticate", `Bearer realm="notes-api"`)
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
