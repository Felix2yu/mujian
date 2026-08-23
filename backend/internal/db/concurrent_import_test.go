package db

import (
	"fmt"
	"mujian/internal/models"
	"sync"
	"testing"
	"time"
)

// Regression for "import record X: database is locked (5) (SQLITE_BUSY)".
//
// A whole import runs as one big write transaction. With the old DEFERRED
// txlock, a second concurrent writer (duplicate import, MCP batch edit, ...)
// could invalidate the running import's read snapshot; SQLite then returns
// SQLITE_BUSY *immediately* (busy_timeout is not consulted on a deferred
// upgrade), failing the import mid-flight. With _txlock=immediate both
// imports serialize on BEGIN and both must succeed.
func TestConcurrentImportsDoNotFailWithBusy(t *testing.T) {
	db := newTestDB(t)

	build := func(tag string, n int) *models.ExportData {
		data := &models.ExportData{}
		for i := 0; i < n; i++ {
			data.Records = append(data.Records, models.Record{
				ID:          fmt.Sprintf("%s-%04d", tag, i),
				Name:        fmt.Sprintf("昆剧经典折子戏 %s %d", tag, i),
				ArtistNames: []string{fmt.Sprintf("演员%s-%d", tag, i%40)},
				CategoryName: "昆剧",
			})
		}
		return data
	}

	const workers = 4
	errs := make(chan error, workers)
	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			_, err := db.ImportData(build(fmt.Sprintf("w%d", w), 200))
			errs <- err
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent import failed after %v: %v", time.Since(start).Round(time.Millisecond), err)
		}
	}

	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM records").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != workers*200 {
		t.Errorf("expected %d records, got %d", workers*200, n)
	}
}
