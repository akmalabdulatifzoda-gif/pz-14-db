// Package redoc отдаёт статическую страницу ReDoc на /redoc.
// Скрипт redoc.standalone.js встроен в бинарник (go:embed),
// поэтому документация работает без доступа к CDN.
package redoc

import (
	_ "embed"
	"net/http"
)

//go:embed redoc.standalone.js
var script []byte

const page = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>Notes API — ReDoc</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>body{margin:0}</style>
</head>
<body>
  <redoc spec-url="/docs/doc.json"></redoc>
  <script src="/redoc/redoc.standalone.js"></script>
</body>
</html>`

// Page — GET /redoc
func Page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// Script — GET /redoc/redoc.standalone.js
func Script(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(script)
}
