package storage

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// DeleteRaw removes one exact key from the bucket (counterpart of PutRaw).
// 用于同步删除 S3 上的备份副键：本地快照被裁剪 / 删除时不要把对象留在桶里。
// 缺失的键视为成功（NoSuchKey），这样「上传失败过、远端本就没有」的快照也
// 能被正常收敛。
func (s *S3Storage) DeleteRaw(key string) error {
	ctx, cancel := opCtx()
	defer cancel()
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

// MigrateStats summarizes one local → S3 migration run.
type MigrateStats struct {
	Total    int   `json:"total"`
	Migrated int   `json:"migrated"`
	Skipped  int   `json:"skipped"`
	Failed   int   `json:"failed"`
	Bytes    int64 `json:"bytes"`
}

// PutRaw uploads bytes under an exact key, preserving the local layout
// (covers/<hash>.<ext> posters plus covers/<hash>.thumb.* thumbnails).
// Unlike SaveCoverBytes it never renames by content hash.
func (s *S3Storage) PutRaw(key string, data []byte) error {
	ctx, cancel := opCtx()
	defer cancel()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		CacheControl: aws.String("public, max-age=2592000, immutable"),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// MigrateLocalToS3 uploads every file in the local covers/ directory (posters
// and thumbnails) to the S3 bucket under the same key. Objects that already
// exist remotely are skipped, so re-runs are idempotent and cheap. Individual
// failures are counted in stats.Failed without aborting the run. The emit
// callback, if non-nil, is invoked after each file with (done, total).
func MigrateLocalToS3(local *LocalStorage, remote *S3Storage, emit func(done, total int)) (MigrateStats, error) {
	keys, err := local.listKeys("covers")
	if err != nil {
		return MigrateStats{}, err
	}
	stats := MigrateStats{Total: len(keys)}
	for i, k := range keys {
		data, rerr := local.ReadCover(k)
		if rerr != nil {
			stats.Failed++
		} else if remote.CoverExists(k) {
			stats.Skipped++
		} else if perr := remote.PutRaw(k, data); perr != nil {
			stats.Failed++
		} else {
			stats.Migrated++
			stats.Bytes += int64(len(data))
		}
		if emit != nil {
			emit(i+1, stats.Total)
		}
	}
	return stats, nil
}
