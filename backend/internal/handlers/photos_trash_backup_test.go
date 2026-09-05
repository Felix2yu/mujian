package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mujian/internal/config"
	"mujian/internal/models"
)

// ---------- 票根/现场照 ----------

func TestRecordPhotosEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	_, b := doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name": "牡丹亭", "date": time.Now().Unix(),
	})
	var rec models.Record
	decodeResp(t, b, &rec)

	// 空列表。
	res, b := doJSON(t, "GET", ts.URL+"/api/records/"+rec.ID+"/photos", nil)
	expectStatus(t, res, 200, "list photos empty")
	var page struct {
		Photos []models.RecordPhoto `json:"photos"`
	}
	decodeResp(t, b, &page)
	if len(page.Photos) != 0 {
		t.Fatalf("expected no photos, got %v", page.Photos)
	}

	// 上传两张图，依次关联。
	keys := uploadCoverKeys(t, ts, 2)
	for _, key := range keys {
		res, b = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos", map[string]string{"key": key})
		expectStatus(t, res, 201, "add photo "+key)
	}

	// 错误分支：坏 body / 空 key / 未知 key。
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos", []byte("{"))
	expectStatus(t, res, 400, "add photo invalid body")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos", map[string]string{"key": ""})
	expectStatus(t, res, 400, "add photo empty key")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos", map[string]string{"key": "nope"})
	expectStatus(t, res, 400, "add photo unknown key")

	// 列表按 sort 返回。
	res, b = doJSON(t, "GET", ts.URL+"/api/records/"+rec.ID+"/photos", nil)
	expectStatus(t, res, 200, "list photos")
	decodeResp(t, b, &page)
	if len(page.Photos) != 2 || page.Photos[0].FileName != keys[0] {
		t.Fatalf("photos: %+v (want keys %v)", page.Photos, keys)
	}

	// 反转顺序。
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos/reorder", map[string]interface{}{
		"ids": []string{page.Photos[1].ID, page.Photos[0].ID},
	})
	expectStatus(t, res, 200, "reorder photos")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/photos/reorder", []byte("not-json"))
	expectStatus(t, res, 400, "reorder photos invalid body")

	res, b = doJSON(t, "GET", ts.URL+"/api/records/"+rec.ID+"/photos", nil)
	decodeResp(t, b, &page)
	if page.Photos[0].FileName != keys[1] {
		t.Fatalf("reorder did not apply: %+v", page.Photos)
	}

	// 删除关联后再清一张；删除不存在的 pid 也不报错。
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec.ID+"/photos/"+page.Photos[0].ID, nil)
	expectStatus(t, res, 200, "delete photo")
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec.ID+"/photos/missing-pid", nil)
	expectStatus(t, res, 200, "delete missing photo is a no-op")
}

// uploadCoverKeys uploads n fixtures through /api/upload and returns keys.
func uploadCoverKeys(t *testing.T, ts *httptest.Server, n int) []string {
	t.Helper()
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		res, b := uploadFile(t, ts.URL+"/api/upload", "file", "cover.jpg", jpgFixture(), "image/jpeg")
		expectStatus(t, res, 200, "upload cover")
		var out struct {
			Key string `json:"key"`
		}
		decodeResp(t, b, &out)
		if out.Key == "" {
			t.Fatalf("upload returned empty key: %s", b)
		}
		keys = append(keys, out.Key)
	}
	return keys
}

// ---------- 回收站 ----------

func TestTrashEndpoints(t *testing.T) {
	ts, _, db, _ := newTestServer(t, nil)

	// 空回收站。
	res, b := doJSON(t, "GET", ts.URL+"/api/records/deleted", nil)
	expectStatus(t, res, 200, "empty trash")
	var page struct {
		Records []models.Record `json:"records"`
		Total   int             `json:"total"`
	}
	decodeResp(t, b, &page)
	if page.Total != 0 || page.Records == nil {
		t.Fatalf("empty trash: %+v", page)
	}

	var rec models.Record
	_, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{"name": "回收测试", "date": time.Now().Unix()})
	decodeResp(t, b, &rec)

	// 软删除进入回收站。
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec.ID, nil)
	expectStatus(t, res, 200, "soft delete")
	res, b = doJSON(t, "GET", ts.URL+"/api/records/deleted?limit=10&offset=0", nil)
	expectStatus(t, res, 200, "list trash")
	decodeResp(t, b, &page)
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].Name != "回收测试" {
		t.Fatalf("trash: %+v", page)
	}

	// 未删除的记录无法恢复（404 分支）。
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/unknown-id/restore", nil)
	expectStatus(t, res, 404, "restore unknown")
	_, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{"name": "还活着", "date": time.Now().Unix()})
	var live models.Record
	decodeResp(t, b, &live)
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/"+live.ID+"/restore", nil)
	expectStatus(t, res, 404, "restore live record")

	// 恢复。
	res, b = doJSON(t, "POST", ts.URL+"/api/records/"+rec.ID+"/restore", nil)
	expectStatus(t, res, 200, "restore")
	var restored models.Record
	decodeResp(t, b, &restored)
	if restored.ID != rec.ID {
		t.Fatalf("restore returned %+v", restored)
	}

	// 再次软删后彻底清除；purge 未知 id 静默成功。
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec.ID, nil)
	expectStatus(t, res, 200, "soft delete again")
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/unknown-id/purge", nil)
	expectStatus(t, res, 200, "purge unknown is a no-op")
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec.ID+"/purge", nil)
	expectStatus(t, res, 200, "purge")
	if n, _ := db.DeletedCount(); n != 0 {
		t.Fatalf("purged record still in trash: %d", n)
	}

	// 清空回收站。
	_, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{"name": "清空测试", "date": time.Now().Unix()})
	var rec2 models.Record
	decodeResp(t, b, &rec2)
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/"+rec2.ID, nil)
	expectStatus(t, res, 200, "soft delete for trash purge")
	res, b = doJSON(t, "POST", ts.URL+"/api/records/trash/purge", nil)
	expectStatus(t, res, 200, "purge trash")
	var purged struct {
		Purged int `json:"purged"`
	}
	decodeResp(t, b, &purged)
	if purged.Purged != 1 {
		t.Fatalf("trash purge: %+v", purged)
	}
	res, b = doJSON(t, "POST", ts.URL+"/api/records/trash/purge", nil)
	expectStatus(t, res, 200, "purge empty trash")
	decodeResp(t, b, &purged)
	if purged.Purged != 0 {
		t.Fatalf("empty trash purge: %+v", purged)
	}
}

// ---------- 备份 ----------

func TestBackupEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, func(c *config.Config) {
		c.BackupFormat = "json"
	})

	// 触发备份。
	res, b := doJSON(t, "POST", ts.URL+"/api/backup/run", nil)
	expectStatus(t, res, 200, "run backup")
	var run struct {
		File string `json:"file"`
	}
	decodeResp(t, b, &run)
	if !strings.HasPrefix(run.File, "mujian-") {
		t.Fatalf("backup file name: %q", run.File)
	}

	// 清单。
	res, b = doJSON(t, "GET", ts.URL+"/api/backup/list", nil)
	expectStatus(t, res, 200, "list backups")
	var list struct {
		Backups []struct {
			Name string `json:"file"`
		} `json:"backups"`
	}
	decodeResp(t, b, &list)
	if len(list.Backups) != 1 || list.Backups[0].Name != run.File {
		t.Fatalf("backup list: %+v", list)
	}

	// 下载。
	res, b = doJSON(t, "GET", ts.URL+"/api/backup/download?file="+run.File, nil)
	expectStatus(t, res, 200, "download backup")
	if len(b) == 0 {
		t.Fatal("download backup returned empty body")
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/backup/download?file=../evil", nil)
	expectStatus(t, res, 400, "download bad name")

	// restore-from 的错误分支。
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/restore-from", []byte("{"))
	expectStatus(t, res, 400, "restore-from invalid body")
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/restore-from", map[string]string{"file": "../evil"})
	expectStatus(t, res, 400, "restore-from bad name")
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/restore-from", map[string]string{"file": "mujian-20260101-000000.db"})
	expectStatus(t, res, 400, "restore-from .db snapshot")
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/restore-from", map[string]string{"file": "mujian-20260101-000000.tar"})
	expectStatus(t, res, 400, "restore-from unsupported ext")

	// 从 json 快照在线恢复。
	res, b = doJSON(t, "POST", ts.URL+"/api/backup/restore-from", map[string]string{"file": run.File})
	expectStatus(t, res, 200, "restore-from json")
	var restored struct {
		Records int `json:"records"`
	}
	decodeResp(t, b, &restored)

	// 删除快照。
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/backup?file="+run.File, nil)
	expectStatus(t, res, 200, "delete backup")
	// 删除不存在的快照是幂等的（os.IsNotExist 归 nil），但坏文件名仍拒绝。
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/backup?file=../evil", nil)
	expectStatus(t, res, 400, "delete bad name")
}

// ---------- 地图 ----------

func TestMapPointsEndpoint(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	res, b := doJSON(t, "GET", ts.URL+"/api/map/points", nil)
	expectStatus(t, res, 200, "map points empty")

	_, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name": "有坐标", "date": time.Now().Unix(),
		"coordinate": map[string]float64{"lat": 31.23, "lng": 121.47},
	})
	var with models.Record
	decodeResp(t, b, &with)
	_, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{"name": "无坐标", "date": time.Now().Unix()})

	res, b = doJSON(t, "GET", ts.URL+"/api/map/points", nil)
	expectStatus(t, res, 200, "map points")
	var pts []map[string]interface{}
	decodeResp(t, b, &pts)
	if len(pts) != 1 {
		t.Fatalf("map points should only include records with coordinates: %v", pts)
	}
}

// ---------- AI 填写 ----------

type aiStub struct {
	srv      *httptest.Server
	requests int
}

// newAIStub starts a fake OpenAI-compatible /chat/completions endpoint.
func newAIStub(t *testing.T, status int, body string) *aiStub {
	t.Helper()
	s := &aiStub{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func aiConfig(url string) func(*config.Config) {
	return func(c *config.Config) {
		c.AIEnabled = true
		c.AIBaseURL = url
		c.AIAPIKey = "test-key"
		c.AIModel = "test-model"
	}
}

func TestParseAI(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	// 未配置 AI。
	res, _ := doJSON(t, "POST", ts.URL+"/api/ai/parse", map[string]string{"text": "x"})
	expectStatus(t, res, 400, "ai unconfigured")

	// 坏 body / 空文本。
	res, _ = doJSON(t, "POST", ts.URL+"/api/ai/parse", []byte("{"))
	expectStatus(t, res, 400, "ai invalid body")
	res, _ = doJSON(t, "POST", ts.URL+"/api/ai/parse", map[string]string{"text": "   "})
	expectStatus(t, res, 400, "ai empty text")
}

func TestParseAISuccess(t *testing.T) {
	stub := newAIStub(t, 200, `{"choices":[{"message":{"content":"{\"name\":\"牡丹亭\",\"city\":\"上海\"}"}}]}`)
	ts, _, _, _ := newTestServer(t, aiConfig(stub.srv.URL))

	res, b := doJSON(t, "POST", ts.URL+"/api/ai/parse", map[string]string{"text": "今晚牡丹亭"})
	expectStatus(t, res, 200, "ai parse success")
	var out map[string]interface{}
	decodeResp(t, b, &out)
	if out["name"] != "牡丹亭" {
		t.Fatalf("ai parse out: %v", out)
	}
	if stub.requests != 1 {
		t.Fatalf("stub called %d times", stub.requests)
	}
}

func TestParseAILLMFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"upstream error", 500, `{"error":"boom"}`},
		{"invalid json envelope", 200, "not-json"},
		{"empty choices", 200, `{"choices":[]}`},
		{"non-json content", 200, `{"choices":[{"message":{"content":"抱歉，我无法解析"}}]}`},
		{"markdown fenced json", 200, "{\"choices\":[{\"message\":{\"content\":\"```json\\n{\\\"name\\\":\\\"牡丹亭\\\"}\\n```\"}}]}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newAIStub(t, tc.status, tc.body)
			ts, _, _, _ := newTestServer(t, aiConfig(stub.srv.URL))
			res, _ := doJSON(t, "POST", ts.URL+"/api/ai/parse", map[string]string{"text": "文本"})
			want := 502
			if tc.name == "markdown fenced json" {
				want = 200
			}
			expectStatus(t, res, want, "ai parse "+tc.name)
		})
	}
}
