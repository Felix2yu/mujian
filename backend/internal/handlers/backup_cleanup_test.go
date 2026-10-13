package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// 批量清理接口：历史快照堆积（调度缺陷留下的上千份）时逐条删除不可行，
// 这两个端点提供「裁剪到最新 N 份」与「清空全部」。
func TestBackupBulkCleanupEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	// 连续 creating three snapshots; 文件名戳按秒区分。
	for i := 0; i < 3; i++ {
		res, _ := doJSON(t, "POST", ts.URL+"/api/backup/run", nil)
		expectStatus(t, res, 200, "run backup "+time.Now().String())
		time.Sleep(1100 * time.Millisecond)
	}
	res, b := doJSON(t, "GET", ts.URL+"/api/backup/list", nil)
	expectStatus(t, res, 200, "list backups")
	var listed struct {
		Backups []struct {
			File string `json:"file"`
		} `json:"backups"`
	}
	if err := json.Unmarshal(b, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Backups) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(listed.Backups))
	}

	// 裁剪到最新 1 份。
	res, b = doJSON(t, "POST", ts.URL+"/api/backup/prune", map[string]int{"keep": 1})
	expectStatus(t, res, 200, "prune backups")
	var pruned struct {
		Deleted   int `json:"deleted"`
		Remaining int `json:"remaining"`
	}
	if err := json.Unmarshal(b, &pruned); err != nil {
		t.Fatal(err)
	}
	if pruned.Deleted != 2 || pruned.Remaining != 1 {
		t.Fatalf("prune keep=1: deleted=%d remaining=%d", pruned.Deleted, pruned.Remaining)
	}

	// 非法 keep 应 400 而不是把目录清空。
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/prune", map[string]int{"keep": -1})
	expectStatus(t, res, http.StatusBadRequest, "negative keep")

	// 清空。
	res, b = doJSON(t, "DELETE", ts.URL+"/api/backup/all", nil)
	expectStatus(t, res, 200, "delete all backups")
	var cleared struct {
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(b, &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Deleted != 1 {
		t.Fatalf("delete all: deleted=%d, want 1", cleared.Deleted)
	}

	res, b = doJSON(t, "GET", ts.URL+"/api/backup/list", nil)
	expectStatus(t, res, 200, "list after cleanup")
	if err := json.Unmarshal(b, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Backups) != 0 {
		t.Fatalf("expected no snapshots after cleanup, got %d", len(listed.Backups))
	}
}
