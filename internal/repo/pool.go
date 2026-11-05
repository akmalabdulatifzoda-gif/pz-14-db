package repo

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig — параметры пула. Значения по умолчанию подобраны
// нагрузочным тестом (см. README / отчёт ПЗ 14).
type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// PoolConfigFromEnv читает настройки с префиксом (DB_ или DB_READ_).
func PoolConfigFromEnv(prefix string, defMax, defMin int) PoolConfig {
	return PoolConfig{
		MaxConns:          int32(envInt(prefix+"MAX_CONNS", defMax)),
		MinConns:          int32(envInt(prefix+"MIN_CONNS", defMin)),
		MaxConnLifetime:   time.Hour,        // ротация: защита от «вечных» соединений и утечек памяти на стороне БД
		MaxConnIdleTime:   5 * time.Minute,  // лишние простаивающие соединения закрываются
		HealthCheckPeriod: 30 * time.Second, // фоновая проверка живости
	}
}

// NewPool создаёт pgxpool и проверяет соединение.
func NewPool(ctx context.Context, url string, pc PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = pc.MaxConns
	cfg.MinConns = pc.MinConns
	cfg.MaxConnLifetime = pc.MaxConnLifetime
	cfg.MaxConnIdleTime = pc.MaxConnIdleTime
	cfg.HealthCheckPeriod = pc.HealthCheckPeriod
	// По умолчанию pgx работает в режиме QueryExecModeCacheStatement:
	// каждый SQL-текст один раз готовится (PREPARE) на соединении и дальше
	// выполняется как prepared statement — без повторного парсинга и планирования.

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// PoolStats — снимок состояния пула для /debug/pool.
type PoolStats struct {
	MaxConns          int32   `json:"maxConns"`
	TotalConns        int32   `json:"totalConns"`
	AcquiredConns     int32   `json:"acquiredConns"`
	IdleConns         int32   `json:"idleConns"`
	AcquireCount      int64   `json:"acquireCount"`
	EmptyAcquireCount int64   `json:"emptyAcquireCount"` // сколько раз пришлось ждать свободное соединение
	CanceledAcquires  int64   `json:"canceledAcquireCount"`
	AcquireWaitMs     float64 `json:"acquireWaitMsTotal"`
}

func Stats(p *pgxpool.Pool) PoolStats {
	s := p.Stat()
	return PoolStats{
		MaxConns:          s.MaxConns(),
		TotalConns:        s.TotalConns(),
		AcquiredConns:     s.AcquiredConns(),
		IdleConns:         s.IdleConns(),
		AcquireCount:      s.AcquireCount(),
		EmptyAcquireCount: s.EmptyAcquireCount(),
		CanceledAcquires:  s.CanceledAcquireCount(),
		AcquireWaitMs:     float64(s.AcquireDuration().Microseconds()) / 1000,
	}
}
