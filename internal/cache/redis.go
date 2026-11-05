// Package cache — кэш заметок в Redis (cache-aside).
package cache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
)

type NoteCache struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(addr string, ttl time.Duration) (*NoteCache, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 50})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &NoteCache{rdb: rdb, ttl: ttl}, nil
}

func key(id int64) string { return "note:" + strconv.FormatInt(id, 10) }

// Get возвращает (note, true) при попадании в кэш.
func (c *NoteCache) Get(ctx context.Context, id int64) (core.Note, bool) {
	b, err := c.rdb.Get(ctx, key(id)).Bytes()
	if err != nil {
		return core.Note{}, false
	}
	var n core.Note
	if json.Unmarshal(b, &n) != nil {
		return core.Note{}, false
	}
	return n, true
}

func (c *NoteCache) Set(ctx context.Context, n core.Note) {
	if b, err := json.Marshal(n); err == nil {
		c.rdb.Set(ctx, key(n.ID), b, c.ttl)
	}
}

// Invalidate удаляет запись после PATCH/DELETE.
func (c *NoteCache) Invalidate(ctx context.Context, id int64) {
	c.rdb.Del(ctx, key(id))
}

func (c *NoteCache) Close() error { return c.rdb.Close() }
