package storage

import (
	"mime/multipart"
	"strings"
	"sync"

	"mujian/internal/config"
)

// Dynamic resolves the storage backend per call from the current config, so
// switching 封面存储方式 (local ↔ S3) or editing S3 credentials in the
// settings page takes effect immediately — no restart. The backend instance
// is cached and rebuilt only when the effective backend identity changes.
//
// 注意：切换后已有封面不会自动搬家——切到 S3 前先执行「把本地封面上传到
// S3」迁移，否则旧图在本地磁盘不可见。
type Dynamic struct {
	mu  sync.Mutex
	cfg *config.Config
	cur Storage
	sig string // 当前缓存的凭据/类型指纹
}

func NewDynamic(cfg *config.Config) *Dynamic {
	return &Dynamic{cfg: cfg}
}

// backendIdentity 唯一确定一个后端实例：类型 + 全部 S3 连接参数。
func backendIdentity(mode string, s3 config.S3Settings) string {
	return strings.Join([]string{mode, s3.Bucket, s3.Endpoint, s3.Region, s3.AccessKey, s3.SecretKey, s3.PublicURL}, "|")
}

func (d *Dynamic) current() Storage {
	d.mu.Lock()
	defer d.mu.Unlock()

	mode, _ := d.cfg.GetStorageMode()
	s3 := d.cfg.GetS3Settings()
	sig := backendIdentity(mode, s3)
	if d.cur != nil && sig == d.sig {
		return d.cur
	}

	var next Storage
	if mode == "s3" && s3.Bucket != "" && s3.AccessKey != "" {
		next = NewS3StorageFromSettings(s3, d.cfg.GetImageFormat)
	} else {
		// 凭据不完整时退回本地存储（与启动时的 New() 行为一致）
		next = NewLocalStorage(d.cfg.UploadDir, d.cfg.GetImageFormat)
	}
	d.cur = next
	d.sig = sig
	return next
}

// ---------- Storage 接口全量转发 ----------

func (d *Dynamic) SaveUpload(file *multipart.FileHeader) (key, thumb string, created bool, err error) {
	return d.current().SaveUpload(file)
}

func (d *Dynamic) SaveCoverBytes(data []byte, ext string) (string, bool, error) {
	return d.current().SaveCoverBytes(data, ext)
}

func (d *Dynamic) ReadCover(key string) ([]byte, error) {
	return d.current().ReadCover(key)
}

func (d *Dynamic) CoverExists(key string) bool {
	return d.current().CoverExists(key)
}

func (d *Dynamic) DeleteCover(key string) error {
	return d.current().DeleteCover(key)
}

func (d *Dynamic) MoveCoverToTrash(key string) error {
	return d.current().MoveCoverToTrash(key)
}

func (d *Dynamic) ListCoverKeys() ([]string, error) {
	return d.current().ListCoverKeys()
}

func (d *Dynamic) ListTrashKeys() ([]string, error) {
	return d.current().ListTrashKeys()
}

func (d *Dynamic) PurgeTrash() (int, error) {
	return d.current().PurgeTrash()
}

func (d *Dynamic) ConvertCover(key, targetFormat string) (newKey string, converted bool, err error) {
	return d.current().ConvertCover(key, targetFormat)
}

func (d *Dynamic) MakeThumbnail(coverKey string, srcData []byte, maxW int, format string) (string, error) {
	return d.current().MakeThumbnail(coverKey, srcData, maxW, format)
}

func (d *Dynamic) ThumbIndex() map[string]string {
	return d.current().ThumbIndex()
}
