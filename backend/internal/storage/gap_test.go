package storage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mujian/internal/config"
)

func jpgTestBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Dynamic 转发链路的其余方法：上传、缩略图、索引、回收站、格式转换。
func TestDynamicForwardedMethods(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{UploadDir: dir, StorageType: "local", Timezone: "UTC", ImageFormat: "png"}
	d := NewDynamic(cfg)

	// SaveUpload：multipart 头（真实 jpeg）→ 返回 key + thumb。
	key, thumb, created, err := d.SaveUpload(multipartFileHeader(t, "file", "cover.jpg", jpgTestBytes(t)))
	if err != nil || !created || key == "" || thumb == "" {
		t.Fatalf("SaveUpload: key=%q thumb=%q created=%v err=%v", key, thumb, created, err)
	}

	// MakeThumbnail 对已有封面再生成一个 64px 缩略图。
	if _, err := d.MakeThumbnail(key, jpgTestBytes(t), 64, "png"); err != nil {
		t.Fatalf("MakeThumbnail: %v", err)
	}

	// ThumbIndex：封面基名（去掉 covers/ 前缀和扩展名）→ 缩略图 key。
	base := key[:strings.LastIndex(key, ".")]
	base = strings.TrimPrefix(base, "covers/")
	idx := d.ThumbIndex()
	if idx[base] == "" {
		t.Fatalf("ThumbIndex missing entry for %s: %v", base, idx)
	}

	// ListCoverKeys 至少包含上传的封面。
	keys, err := d.ListCoverKeys()
	if err != nil {
		t.Fatalf("ListCoverKeys: %v", err)
	}
	found := false
	for _, k := range keys {
		if k == key {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListCoverKeys missing %s: %v", key, keys)
	}

	// ConvertCover：png → jpg 换格式存储。
	newKey, converted, err := d.ConvertCover(key, "jpg")
	if err != nil || !converted {
		t.Fatalf("ConvertCover: key=%q converted=%v err=%v", newKey, converted, err)
	}
	if newKey == key {
		t.Fatal("convert should produce a new key")
	}

	// MoveCoverToTrash → ListTrashKeys → PurgeTrash。
	if err := d.MoveCoverToTrash(newKey); err != nil {
		t.Fatalf("MoveCoverToTrash: %v", err)
	}
	if d.CoverExists(newKey) {
		t.Fatal("cover should be gone from covers/")
	}
	trashKeys, err := d.ListTrashKeys()
	if err != nil {
		t.Fatalf("ListTrashKeys: %v", err)
	}
	if len(trashKeys) == 0 {
		t.Fatalf("trash should not be empty: %v", trashKeys)
	}
	n, err := d.PurgeTrash()
	if err != nil || n != len(trashKeys) {
		t.Fatalf("PurgeTrash: n=%d err=%v (trashKeys=%d)", n, err, len(trashKeys))
	}
	if lenTrashKeys(t, d) != 0 {
		t.Fatal("trash should be empty after purge")
	}

	// DeleteCover 直接删除。
	k2, _, err := d.SaveCoverBytes(jpgTestBytes(t), ".jpg")
	if err != nil {
		t.Fatalf("SaveCoverBytes: %v", err)
	}
	if err := d.DeleteCover(k2); err != nil {
		t.Fatalf("DeleteCover: %v", err)
	}
	if d.CoverExists(k2) {
		t.Fatal("cover should be deleted")
	}
}

func lenTrashKeys(t *testing.T, d *Dynamic) int {
	t.Helper()
	keys, err := d.ListTrashKeys()
	if err != nil {
		t.Fatalf("ListTrashKeys: %v", err)
	}
	return len(keys)
}

// TestConnection 用 fake S3 验证写探测 + 清理，以及缺配置时的报错分支。
func TestS3TestConnection(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()

	newStorage := func(bucket string) *S3Storage {
		return NewS3StorageFromSettings(config.S3Settings{
			Endpoint: ts.URL, Bucket: bucket, Region: "us-east-1",
			AccessKey: "ak", SecretKey: "sk",
		}, func() string { return "avif" })
	}

	// 正常：写入探测对象后清理。
	if err := newStorage("mujian-test").TestConnection(context.Background()); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if len(fs.objects) != 0 {
		t.Fatalf("probe object should be deleted, left: %v", fs.objects)
	}

	// bucket 未配置。
	if err := newStorage("").TestConnection(context.Background()); err == nil {
		t.Fatal("empty bucket should error")
	}
}

// TestDynamicSwitchesToS3 exercises the full-credentials branch of
// Dynamic.current(): the rebuilt backend forwards to S3.
func TestDynamicSwitchesToS3(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()

	var mu sync.Mutex
	cfg := &config.Config{UploadDir: t.TempDir(), StorageType: "local", Timezone: "UTC"}
	d := NewDynamic(cfg)

	mu.Lock()
	cfg.StorageType = "s3"
	cfg.S3Endpoint = ts.URL
	cfg.S3Bucket = "mujian-test"
	cfg.S3Region = "us-east-1"
	cfg.S3AccessKey = "ak"
	cfg.S3SecretKey = "sk"
	mu.Unlock()

	if _, created, err := d.SaveCoverBytes([]byte("s3-data"), ".png"); err != nil || !created {
		t.Fatalf("save via s3 backend: %v", err)
	}
	keys, err := d.ListCoverKeys()
	if err != nil || len(keys) == 0 {
		t.Fatalf("ListCoverKeys via s3: %v %v", keys, err)
	}
	if !d.CoverExists(keys[0]) {
		t.Fatal("CoverExists via s3 should be true")
	}
	idx := d.ThumbIndex()
	if idx == nil {
		t.Fatal("ThumbIndex via s3 should not be nil")
	}
}
