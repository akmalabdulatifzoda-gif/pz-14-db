// Утилита для эксперимента: "created_at|id" из psql -> курсор keyset-пагинации.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core"
	"github.com/akmalabdulatifzoda-gif/pz-14-db/internal/core/service"
)

func main() {
	ts, id, _ := strings.Cut(os.Args[1], "|")
	t, err := time.Parse("2006-01-02 15:04:05.999999-07", ts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	n, _ := strconv.ParseInt(id, 10, 64)
	fmt.Println(service.EncodeCursor(core.Cursor{CreatedAt: t, ID: n}))
}
