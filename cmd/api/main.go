// Package main Notes API server.
//
//	@title						Notes API
//	@version					1.2
//	@description				Учебный REST API для заметок. ПЗ №14 — PostgreSQL, connection pool, оптимизация запросов.
//	@description				Методы POST, PATCH и DELETE требуют Bearer-токен, если сервер запущен с API_TOKEN.
//	@contact.name				Абдулатифзода Акмал, ЭФМО-0126
//	@contact.url				https://github.com/akmalabdulatifzoda-gif/pz-14-db
//	@license.name				MIT
//	@license.url				https://opensource.org/licenses/MIT
//	@BasePath					/api/v1
//	@schemes					http
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Введите токен в формате: Bearer ВАШ_ТОКЕН (для демо: Bearer secret)
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "github.com/akmalabdulatifzoda-gif/pz-14-db/docs" // сгенерированная спецификация (swag init)
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/cache"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core/service"
	httpx "github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/http/handlers"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/repo"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()
	writeURL := getenv("DATABASE_URL", "postgres://notes:pass@localhost:5432/notes?sslmode=disable")
	readURL := getenv("DATABASE_READ_URL", writeURL) // адрес реплики; по умолчанию тот же сервер

	// Пул записи: короткие транзакции, небольшая доля трафика.
	wcfg := repo.PoolConfigFromEnv("DB_", 5, 1)
	// Пул чтения: основная нагрузка. 10 — оптимум нагрузочного теста на стенде 2 vCPU
	// (максимум RPS и минимальный p99; меньше — очередь за соединением, больше — конкуренция за CPU БД).
	rcfg := repo.PoolConfigFromEnv("DB_READ_", 10, 2)

	wpool, err := repo.NewPool(ctx, writeURL, wcfg)
	if err != nil {
		log.Fatal(err)
	}
	defer wpool.Close()
	rpool, err := repo.NewPool(ctx, readURL, rcfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rpool.Close()
	log.Printf("Go %s, GOMAXPROCS=%d", runtime.Version(), runtime.GOMAXPROCS(0))
	log.Printf("pool write: max=%d min=%d; pool read: max=%d min=%d",
		wcfg.MaxConns, wcfg.MinConns, rcfg.MaxConns, rcfg.MinConns)

	var c service.Cache
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		nc, err := cache.New(addr, 60*time.Second)
		if err != nil {
			log.Fatalf("redis: %v", err)
		}
		defer nc.Close()
		c = nc
		log.Println("Redis cache enabled:", addr, "TTL 60s")
	}

	svc := service.NewNoteService(repo.NewNoteRepoPG(wpool, rpool), c)
	h := &handlers.Handler{Svc: svc}
	r := httpx.NewRouter(h, httpx.Options{
		APIToken:       os.Getenv("API_TOKEN"),
		RequestTimeout: 2 * time.Second,
		AccessLog:      os.Getenv("ACCESS_LOG") == "1",
		PoolStats: func() any {
			return map[string]repo.PoolStats{"write": repo.Stats(wpool), "read": repo.Stats(rpool)}
		},
	})

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Println("Server started at", addr, "| Swagger UI: /docs/index.html | pool stats: /debug/pool")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Корректное завершение: дожидаемся запросов и закрываем пулы.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	log.Println("server stopped")
}
