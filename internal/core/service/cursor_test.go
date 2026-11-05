package service

import (
	"testing"
	"time"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
)

func TestCursorRoundTrip(t *testing.T) {
	c := core.Cursor{CreatedAt: time.Date(2026, 11, 5, 12, 0, 0, 123456000, time.UTC), ID: 42}
	got, err := DecodeCursor(EncodeCursor(c))
	if err != nil || !got.CreatedAt.Equal(c.CreatedAt) || got.ID != c.ID {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if _, err := DecodeCursor("bad!"); err == nil {
		t.Fatal("expected error")
	}
}
