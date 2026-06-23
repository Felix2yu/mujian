package config

import (
	"encoding/json"
	"os"
	"sync"
)

type Config struct {
	AllowLocalStorage bool   `json:"-"`
	DBPath            string `json:"-"`
	UploadDir         string `json:"-"`
	Port              string `json:"-"`
	Theme             string `json:"theme"`
	StorageType       string `json:"storage_type"`
	S3Endpoint        string `json:"s3_endpoint"`
	S3Bucket          string `json:"s3_bucket"`
	S3Region          string `json:"s3_region"`
	S3AccessKey       string `json:"s3_access_key"`
	S3SecretKey       string `json:"s3_secret_key"`
	S3PublicURL       string `json:"s3_public_url"`
	mu                sync.RWMutex
}

var (
	global *Config
)

func Load() *Config {
	global = &Config{
		AllowLocalStorage: os.Getenv("ALLOW_LOCAL_STORAGE") != "false",
		DBPath:            getEnv("DB_PATH", "./data/mujian.db"),
		UploadDir:         getEnv("UPLOAD_DIR", "./data/uploads"),
		Port:              getEnv("PORT", "8080"),
		Theme:             getEnv("THEME", "auto"),
		StorageType:       getEnv("STORAGE_TYPE", "local"),
		S3Endpoint:        os.Getenv("S3_ENDPOINT"),
		S3Bucket:          os.Getenv("S3_BUCKET"),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		S3AccessKey:       os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:       os.Getenv("S3_SECRET_KEY"),
		S3PublicURL:       os.Getenv("S3_PUBLIC_URL"),
	}
	return global
}

func Get() *Config {
	return global
}

func (c *Config) Update(s *SettingsUpdate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s.Theme != nil {
		c.Theme = *s.Theme
	}
	if s.StorageType != nil && c.AllowLocalStorage || *s.StorageType == "s3" {
		c.StorageType = *s.StorageType
	}
	if s.S3Endpoint != nil {
		c.S3Endpoint = *s.S3Endpoint
	}
	if s.S3Bucket != nil {
		c.S3Bucket = *s.S3Bucket
	}
	if s.S3Region != nil {
		c.S3Region = *s.S3Region
	}
	if s.S3AccessKey != nil {
		c.S3AccessKey = *s.S3AccessKey
	}
	if s.S3SecretKey != nil {
		c.S3SecretKey = *s.S3SecretKey
	}
	if s.S3PublicURL != nil {
		c.S3PublicURL = *s.S3PublicURL
	}
}

type SettingsUpdate struct {
	Theme       *string `json:"theme"`
	StorageType *string `json:"storage_type"`
	S3Endpoint  *string `json:"s3_endpoint"`
	S3Bucket    *string `json:"s3_bucket"`
	S3Region    *string `json:"s3_region"`
	S3AccessKey *string `json:"s3_access_key"`
	S3SecretKey *string `json:"s3_secret_key"`
	S3PublicURL *string `json:"s3_public_url"`
}

func (c *Config) GetSettingsResponse() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s3Key := c.S3SecretKey
	if len(s3Key) > 4 {
		s3Key = s3Key[:4] + "****"
	}

	return map[string]interface{}{
		"theme":              c.Theme,
		"storage_type":       c.StorageType,
		"allow_local_storage": c.AllowLocalStorage,
		"s3_endpoint":        c.S3Endpoint,
		"s3_bucket":          c.S3Bucket,
		"s3_region":          c.S3Region,
		"s3_access_key":      c.S3AccessKey,
		"s3_secret_key":      s3Key,
		"s3_public_url":      c.S3PublicURL,
	}
}

func (c *Config) SaveToFile(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data := map[string]string{
		"theme":        c.Theme,
		"storage_type": c.StorageType,
		"s3_endpoint":  c.S3Endpoint,
		"s3_bucket":    c.S3Bucket,
		"s3_region":    c.S3Region,
		"s3_access_key": c.S3AccessKey,
		"s3_secret_key": c.S3SecretKey,
		"s3_public_url": c.S3PublicURL,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func (c *Config) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data map[string]string
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := data["theme"]; ok {
		c.Theme = v
	}
	if v, ok := data["storage_type"]; ok {
		if c.AllowLocalStorage || v == "s3" {
			c.StorageType = v
		}
	}
	if v, ok := data["s3_endpoint"]; ok {
		c.S3Endpoint = v
	}
	if v, ok := data["s3_bucket"]; ok {
		c.S3Bucket = v
	}
	if v, ok := data["s3_region"]; ok {
		c.S3Region = v
	}
	if v, ok := data["s3_access_key"]; ok {
		c.S3AccessKey = v
	}
	if v, ok := data["s3_secret_key"]; ok {
		c.S3SecretKey = v
	}
	if v, ok := data["s3_public_url"]; ok {
		c.S3PublicURL = v
	}

	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
