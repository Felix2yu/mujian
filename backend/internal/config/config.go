package config

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	AllowLocalStorage bool   `json:"-"`
	DBPath            string `json:"-"`
	UploadDir         string `json:"-"`
	Port              string `json:"-"`
	Timezone          string `json:"-"`
	// AuthToken enables optional bearer-token auth for /api and /mcp when
	// non-empty. It is read via AuthTokenValue() (RLock'd) and never echoed
	// back by GetSettingsResponse.
	AuthToken     string `json:"-"`
	Theme         string `json:"theme"`
	StorageType   string `json:"storage_type"`
	ImageFormat   string `json:"image_format"` // avif | webp | jpeg
	S3Endpoint    string `json:"s3_endpoint"`
	S3Bucket      string `json:"s3_bucket"`
	S3Region      string `json:"s3_region"`
	S3AccessKey   string `json:"s3_access_key"`
	S3SecretKey   string `json:"s3_secret_key"`
	S3PublicURL   string `json:"s3_public_url"`
	ShowFriends   bool   `json:"show_friends"`
	ShowPayPrice  bool   `json:"show_pay_price"`
	ShowOtherCost bool   `json:"show_other_cost"`
	MultiCurrency bool   `json:"multi_currency"`
	mu            sync.RWMutex
}

var (
	global *Config
)

func Load() *Config {
	loc := loadTimezone(getEnv("TZ", "Asia/Shanghai"))
	global = &Config{
		AllowLocalStorage: os.Getenv("ALLOW_LOCAL_STORAGE") != "false",
		DBPath:            getEnv("DB_PATH", "./data/mujian.db"),
		UploadDir:         getEnv("UPLOAD_DIR", "./data/uploads"),
		Port:              getEnv("PORT", "8080"),
		Timezone:          loc.String(),
		AuthToken:         os.Getenv("MJ_AUTH_TOKEN"),
		Theme:             getEnv("THEME", "auto"),
		StorageType:       getEnv("STORAGE_TYPE", "local"),
		ImageFormat:       getEnv("IMAGE_FORMAT", "avif"),
		S3Endpoint:        os.Getenv("S3_ENDPOINT"),
		S3Bucket:          os.Getenv("S3_BUCKET"),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		S3AccessKey:       os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:       os.Getenv("S3_SECRET_KEY"),
		S3PublicURL:       os.Getenv("S3_PUBLIC_URL"),
		ShowFriends:       true,
		ShowPayPrice:      true,
		ShowOtherCost:     true,
		MultiCurrency:     true,
	}
	return global
}

func (c *Config) Location() *time.Location {
	return loadTimezone(c.Timezone)
}

func loadTimezone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
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
	if s.StorageType != nil {
		if *s.StorageType == "s3" || c.AllowLocalStorage {
			c.StorageType = *s.StorageType
		}
	}
	if s.ImageFormat != nil {
		c.ImageFormat = *s.ImageFormat
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
		// GET /api/settings masks this value; a client echoing the masked
		// value back must not overwrite the real key.
		if !strings.HasSuffix(*s.S3AccessKey, "****") {
			c.S3AccessKey = *s.S3AccessKey
		}
	}
	if s.S3SecretKey != nil {
		// GET /api/settings masks the secret (e.g. "sk12****"); a client that
		// echoes the masked value back must not overwrite the real key.
		if !strings.HasSuffix(*s.S3SecretKey, "****") {
			c.S3SecretKey = *s.S3SecretKey
		}
	}
	if s.S3PublicURL != nil {
		c.S3PublicURL = *s.S3PublicURL
	}
	if s.ShowFriends != nil {
		c.ShowFriends = *s.ShowFriends
	}
	if s.ShowPayPrice != nil {
		c.ShowPayPrice = *s.ShowPayPrice
	}
	if s.ShowOtherCost != nil {
		c.ShowOtherCost = *s.ShowOtherCost
	}
	if s.MultiCurrency != nil {
		c.MultiCurrency = *s.MultiCurrency
	}
	if s.AuthToken != nil {
		c.AuthToken = *s.AuthToken
	}
}

type SettingsUpdate struct {
	Theme         *string `json:"theme"`
	StorageType   *string `json:"storage_type"`
	ImageFormat   *string `json:"image_format"`
	S3Endpoint    *string `json:"s3_endpoint"`
	S3Bucket      *string `json:"s3_bucket"`
	S3Region      *string `json:"s3_region"`
	S3AccessKey   *string `json:"s3_access_key"`
	S3SecretKey   *string `json:"s3_secret_key"`
	S3PublicURL   *string `json:"s3_public_url"`
	ShowFriends   *bool   `json:"show_friends"`
	ShowPayPrice  *bool   `json:"show_pay_price"`
	ShowOtherCost *bool   `json:"show_other_cost"`
	MultiCurrency *bool   `json:"multi_currency"`
	AuthToken     *string `json:"auth_token"`
}

// maskSecret masks a credential for GET /api/settings: it never returns the
// full value, and always ends in "****" so Update() can recognize an echoed
// masked value and skip overwriting the real key.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 4 {
		return s[:4] + "****"
	}
	return "****"
}

func (c *Config) GetSettingsResponse() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"theme":               c.Theme,
		"storage_type":        c.StorageType,
		"image_format":        c.ImageFormat,
		"allow_local_storage": c.AllowLocalStorage,
		"s3_endpoint":         c.S3Endpoint,
		"s3_bucket":           c.S3Bucket,
		"s3_region":           c.S3Region,
		"s3_access_key":       maskSecret(c.S3AccessKey),
		"s3_secret_key":       maskSecret(c.S3SecretKey),
		"s3_public_url":       c.S3PublicURL,
		"show_friends":        c.ShowFriends,
		"show_pay_price":      c.ShowPayPrice,
		"show_other_cost":     c.ShowOtherCost,
		"multi_currency":      c.MultiCurrency,
		"auth_required":       c.AuthToken != "",
	}
}

// ---------- RLock'd accessors for fields mutated by PUT /api/settings ----------
//
// Handlers and the storage layer must read these through the accessors rather
// than touching the fields directly: a settings update running concurrently
// with any request would otherwise be a data race.

// GetImageFormat returns the preferred cover image format (avif/webp/jpeg).
func (c *Config) GetImageFormat() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ImageFormat
}

// GetStorageMode returns the storage backend selection and whether switching
// away from S3 back to local storage is permitted.
func (c *Config) GetStorageMode() (storageType string, allowLocal bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.StorageType, c.AllowLocalStorage
}

// S3Settings is a point-in-time snapshot of the S3 connection parameters.
type S3Settings struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	PublicURL string
}

// GetS3Settings snapshots the S3 configuration under the read lock.
func (c *Config) GetS3Settings() S3Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return S3Settings{
		Endpoint:  c.S3Endpoint,
		Bucket:    c.S3Bucket,
		Region:    c.S3Region,
		AccessKey: c.S3AccessKey,
		SecretKey: c.S3SecretKey,
		PublicURL: c.S3PublicURL,
	}
}

// AuthTokenValue returns the current bearer token under the read lock.
func (c *Config) AuthTokenValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AuthToken
}

func (c *Config) SaveToFile(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data := map[string]string{
		"theme":           c.Theme,
		"storage_type":    c.StorageType,
		"image_format":    c.ImageFormat,
		"s3_endpoint":     c.S3Endpoint,
		"s3_bucket":       c.S3Bucket,
		"s3_region":       c.S3Region,
		"s3_access_key":   c.S3AccessKey,
		"s3_secret_key":   c.S3SecretKey,
		"s3_public_url":   c.S3PublicURL,
		"show_friends":    b2s(c.ShowFriends),
		"show_pay_price":  b2s(c.ShowPayPrice),
		"show_other_cost": b2s(c.ShowOtherCost),
		"multi_currency":  b2s(c.MultiCurrency),
		"auth_token":      c.AuthToken,
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
	if v, ok := data["image_format"]; ok {
		c.ImageFormat = v
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
	if v, ok := data["show_friends"]; ok {
		c.ShowFriends = v == "true"
	}
	if v, ok := data["show_pay_price"]; ok {
		c.ShowPayPrice = v == "true"
	}
	if v, ok := data["show_other_cost"]; ok {
		c.ShowOtherCost = v == "true"
	}
	if v, ok := data["multi_currency"]; ok {
		c.MultiCurrency = v == "true"
	}
	if v, ok := data["auth_token"]; ok {
		c.AuthToken = v
	}

	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func b2s(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
