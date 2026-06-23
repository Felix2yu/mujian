package storage

import (
	"context"
	"fmt"
	"io"
	"mujian/internal/config"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage interface {
	Save(file *multipart.FileHeader, filename string) (string, error)
	Delete(url string) error
}

type LocalStorage struct {
	uploadDir string
}

type S3Storage struct {
	client    *s3.Client
	bucket    string
	publicURL string
}

func New(cfg *config.Config) Storage {
	if cfg.StorageType == "s3" && cfg.S3Bucket != "" && cfg.S3AccessKey != "" {
		return NewS3Storage(cfg)
	}
	return NewLocalStorage(cfg.UploadDir)
}

func NewLocalStorage(uploadDir string) *LocalStorage {
	os.MkdirAll(uploadDir, 0755)
	return &LocalStorage{uploadDir: uploadDir}
}

func (s *LocalStorage) Save(file *multipart.FileHeader, filename string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	day := time.Now().Format("2006/01/02")
	dir := filepath.Join(s.uploadDir, day)
	os.MkdirAll(dir, 0755)

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = filepath.Ext(file.Filename)
	}
	name := fmt.Sprintf("%s%s", filename, ext)
	path := filepath.Join(dir, name)

	dst, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return "/uploads/" + day + "/" + name, nil
}

func (s *LocalStorage) Delete(url string) error {
	if len(url) > 9 && url[:9] == "/uploads/" {
		path := filepath.Join(s.uploadDir, url[9:])
		return os.Remove(path)
	}
	return nil
}

func NewS3Storage(cfg *config.Config) *S3Storage {
	creds := credentials.NewStaticCredentialsProvider(
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		"",
	)

	client := s3.New(s3.Options{
		Region:       cfg.S3Region,
		BaseEndpoint: aws.String(cfg.S3Endpoint),
		Credentials:  creds,
	})

	return &S3Storage{
		client:    client,
		bucket:    cfg.S3Bucket,
		publicURL: cfg.S3PublicURL,
	}
}

func (s *S3Storage) Save(file *multipart.FileHeader, filename string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	day := time.Now().Format("2006/01/02")
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = filepath.Ext(file.Filename)
	}
	key := fmt.Sprintf("posters/%s/%s%s", day, filename, ext)

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        src,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3 upload: %w", err)
	}

	if s.publicURL != "" {
		return s.publicURL + "/" + key, nil
	}
	return fmt.Sprintf("s3://%s/%s", s.bucket, key), nil
}

func (s *S3Storage) Delete(url string) error {
	if len(url) < 5 || url[:5] != "s3://" {
		return nil
	}
	key := url[5:]
	for i := 5; i < len(url); i++ {
		if url[i] == '/' {
			key = url[i+1:]
			break
		}
	}

	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}
