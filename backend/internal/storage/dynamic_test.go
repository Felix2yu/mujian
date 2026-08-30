package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mujian/internal/config"
)

// 热切换：Dynamic 按当前配置解析后端。本地模式下读写正常；切到「凭据不
// 完整的 S3」时按启动语义退回本地存储，切换过程读写不中断。
func TestDynamicLocalStorage(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{UploadDir: dir, StorageType: "local", Timezone: "UTC"}
	d := NewDynamic(cfg)

	data := []byte("fake-image-bytes")
	key, created, err := d.SaveCoverBytes(data, ".png")
	if err != nil || !created {
		t.Fatalf("save: key=%q created=%v err=%v", key, created, err)
	}
	if got, err := d.ReadCover(key); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read: err=%v len=%d", err, len(got))
	}
	if !d.CoverExists(key) {
		t.Error("cover should exist")
	}
	// 本地模式：文件应真实落在 uploads/covers/ 下
	if _, err := os.Stat(filepath.Join(dir, key)); err != nil {
		t.Errorf("file should exist on local disk: %v", err)
	}
}

func TestDynamicFallsBackToLocalOnIncompleteS3(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{UploadDir: dir, StorageType: "s3", Timezone: "UTC"} // 凭据为空
	d := NewDynamic(cfg)

	key, created, err := d.SaveCoverBytes([]byte("x"), ".png")
	if err != nil || !created {
		t.Fatalf("fallback save failed: %v", err)
	}
	if got, err := d.ReadCover(key); err != nil || string(got) != "x" {
		t.Fatalf("fallback read: %v", err)
	}
	// 回退后仍写本地磁盘
	if _, err := os.Stat(filepath.Join(dir, key)); err != nil {
		t.Errorf("fallback should write to local disk: %v", err)
	}
}

// 配置变更（含凭据编辑）后按新配置重建后端；回退本地时已有数据不受影响。
func TestDynamicRebuildsOnConfigChange(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{UploadDir: dir, StorageType: "local", Timezone: "UTC"}
	d := NewDynamic(cfg)

	key, _, err := d.SaveCoverBytes([]byte("keep-me"), ".png")
	if err != nil {
		t.Fatal(err)
	}

	// 切换存储方式（S3 凭据不完整 → 仍回退本地），已写入的封面应继续可读
	cfg.StorageType = "s3"
	if _, _, err := d.SaveCoverBytes([]byte("y"), ".png"); err != nil {
		t.Fatalf("after switch save: %v", err)
	}
	if got, err := d.ReadCover(key); err != nil || string(got) != "keep-me" {
		t.Fatalf("old cover after switch: err=%v got=%q", err, got)
	}
}
