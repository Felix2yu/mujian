package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("ALLOW_LOCAL_STORAGE", "true")
	t.Setenv("DB_PATH", "/tmp/x.db")
	t.Setenv("UPLOAD_DIR", "/tmp/x")
	t.Setenv("PORT", "9999")
	t.Setenv("THEME", "dark")
	t.Setenv("STORAGE_TYPE", "local")
	t.Setenv("IMAGE_FORMAT", "webp")
	t.Setenv("S3_REGION", "cn-north-1")

	c := Load()
	if c == nil {
		t.Fatal("Load returned nil")
	}
	if c.Port != "9999" || c.DBPath != "/tmp/x.db" || c.UploadDir != "/tmp/x" {
		t.Errorf("env fields not applied: %+v", c)
	}
	if c.Theme != "dark" || c.ImageFormat != "webp" || c.StorageType != "local" {
		t.Errorf("settings not applied: %+v", c)
	}
	if !c.AllowLocalStorage {
		t.Error("AllowLocalStorage should be true")
	}
	if c.Timezone != "Asia/Shanghai" {
		t.Errorf("timezone not loaded: %q", c.Timezone)
	}
	if c.S3Region != "cn-north-1" {
		t.Errorf("s3 region: %q", c.S3Region)
	}

	if Get() != c {
		t.Error("Get() should return the loaded config")
	}
	if loc := c.Location(); loc == nil || loc.String() != "Asia/Shanghai" {
		t.Errorf("Location(): %v", loc)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TZ", "")
	t.Setenv("ALLOW_LOCAL_STORAGE", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("PORT", "")
	c := Load()
	if c.Port != "8080" || c.DBPath != "./data/mujian.db" {
		t.Errorf("defaults wrong: port=%q db=%q", c.Port, c.DBPath)
	}
	if c.AllowLocalStorage == false {
		t.Error("ALLOW_LOCAL_STORAGE unset should default to true")
	}
	if c.Timezone == "" || c.Location() == nil {
		t.Error("default timezone should resolve")
	}
}

func TestLoadTimezoneInvalid(t *testing.T) {
	loc := loadTimezone("Not/AZone")
	if loc != time.UTC {
		t.Errorf("invalid timezone should fall back to UTC, got %v", loc)
	}
}

func strptr(s string) *string { return &s }

func TestUpdate(t *testing.T) {
	c := &Config{AllowLocalStorage: true}
	theme := "light"
	storage := "s3"
	format := "jpeg"
	endpoint := "https://s3.example.com"
	bucket := "bkt"
	region := "us-west-2"
	ak := "AK"
	sk := "SK"
	pub := "https://cdn.example.com"
	c.Update(&SettingsUpdate{
		Theme: &theme, StorageType: &storage, ImageFormat: &format,
		S3Endpoint: &endpoint, S3Bucket: &bucket, S3Region: &region,
		S3AccessKey: &ak, S3SecretKey: &sk, S3PublicURL: &pub,
	})
	if c.Theme != "light" || c.StorageType != "s3" || c.ImageFormat != "jpeg" {
		t.Errorf("update not applied: %+v", c)
	}
	if c.S3Endpoint != endpoint || c.S3Bucket != bucket || c.S3Region != region {
		t.Errorf("s3 update not applied: %+v", c)
	}
	if c.S3AccessKey != ak || c.S3SecretKey != sk || c.S3PublicURL != pub {
		t.Errorf("s3 creds not applied: %+v", c)
	}

	// Disallowed storage type (local storage disabled, target not s3).
	c2 := &Config{AllowLocalStorage: false}
	local := "local"
	c2.Update(&SettingsUpdate{StorageType: &local})
	if c2.StorageType != "" {
		t.Errorf("storage type should be rejected when local storage disabled: %q", c2.StorageType)
	}
	// s3 always allowed even without local storage.
	c2.Update(&SettingsUpdate{StorageType: &storage})
	if c2.StorageType != "s3" {
		t.Errorf("s3 should be allowed: %q", c2.StorageType)
	}

	// Empty update changes nothing.
	c3 := &Config{Theme: "auto"}
	c3.Update(&SettingsUpdate{})
	if c3.Theme != "auto" {
		t.Error("empty update should not change theme")
	}

	// Echoing the masked secret back must not clobber the real key.
	c4 := &Config{S3SecretKey: "real-secret"}
	masked := "real****"
	c4.Update(&SettingsUpdate{S3SecretKey: &masked})
	if c4.S3SecretKey != "real-secret" {
		t.Errorf("masked secret should be ignored: %q", c4.S3SecretKey)
	}
	cleared := ""
	c4.Update(&SettingsUpdate{S3SecretKey: &cleared})
	if c4.S3SecretKey != "" {
		t.Errorf("empty secret should clear the key: %q", c4.S3SecretKey)
	}
}

func TestGetSettingsResponse(t *testing.T) {
	c := &Config{
		Theme: "auto", StorageType: "s3", ImageFormat: "avif",
		AllowLocalStorage: true, S3SecretKey: "supersecret123",
	}
	r := c.GetSettingsResponse()
	if r["theme"] != "auto" || r["storage_type"] != "s3" || r["image_format"] != "avif" {
		t.Errorf("response fields wrong: %v", r)
	}
	if r["s3_secret_key"] != "supe****" {
		t.Errorf("secret should be masked: %v", r["s3_secret_key"])
	}

	// Short secrets must be fully masked too (they used to leak verbatim).
	c2 := &Config{S3SecretKey: "abc"}
	r2 := c2.GetSettingsResponse()
	if r2["s3_secret_key"] != "****" {
		t.Errorf("short secret should be fully masked: %v", r2["s3_secret_key"])
	}

	// Access keys must never be returned in plaintext.
	c3 := &Config{S3AccessKey: "AKIAEXAMPLE"}
	r3 := c3.GetSettingsResponse()
	if r3["s3_access_key"] != "AKIA****" {
		t.Errorf("access key should be masked: %v", r3["s3_access_key"])
	}
	if r3["auth_required"] != false {
		t.Errorf("auth_required should default to false: %v", r3["auth_required"])
	}
}

func TestSaveAndLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	c := &Config{
		Theme: "dark", StorageType: "local", ImageFormat: "webp",
		AllowLocalStorage: true,
		S3Endpoint:        "https://e", S3Bucket: "b", S3Region: "r",
		S3AccessKey: "ak", S3SecretKey: "sk", S3PublicURL: "p",
	}
	if err := c.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	c2 := &Config{AllowLocalStorage: true}
	if err := c2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if c2.Theme != "dark" || c2.ImageFormat != "webp" || c2.S3Bucket != "b" {
		t.Errorf("loaded config mismatch: %+v", c2)
	}

	// Missing file is not an error.
	if err := c2.LoadFromFile(filepath.Join(dir, "nope.json")); err != nil {
		t.Errorf("missing file should be nil error, got %v", err)
	}

	// Invalid JSON is an error.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c2.LoadFromFile(bad); err == nil {
		t.Error("invalid JSON should error")
	}

	// Storage type gating on load.
	c3 := &Config{AllowLocalStorage: false}
	file3 := filepath.Join(dir, "s3.json")
	_ = os.WriteFile(file3, []byte(`{"storage_type":"local","theme":"x"}`), 0600)
	if err := c3.LoadFromFile(file3); err != nil {
		t.Fatal(err)
	}
	if c3.StorageType != "" {
		t.Errorf("local storage should be rejected on load: %q", c3.StorageType)
	}
	_ = os.WriteFile(file3, []byte(`{"storage_type":"s3"}`), 0600)
	if err := c3.LoadFromFile(file3); err != nil {
		t.Fatal(err)
	}
	if c3.StorageType != "s3" {
		t.Errorf("s3 should be accepted: %q", c3.StorageType)
	}
}

func TestAuthTokenLifecycle(t *testing.T) {
	c := &Config{}
	r := c.GetSettingsResponse()
	if r["auth_required"] != false {
		t.Errorf("auth_required should be false without a token: %v", r["auth_required"])
	}

	// Update sets the token; the response must flag it but never echo it.
	tok := "s3cr3t-token"
	c.Update(&SettingsUpdate{AuthToken: &tok})
	if c.AuthTokenValue() != "s3cr3t-token" {
		t.Errorf("AuthTokenValue: got %q", c.AuthTokenValue())
	}
	r = c.GetSettingsResponse()
	if r["auth_required"] != true {
		t.Errorf("auth_required should be true after Update: %v", r["auth_required"])
	}
	for k, v := range r {
		if s, ok := v.(string); ok && s == tok {
			t.Errorf("token leaked via settings field %q", k)
		}
	}

	// Token round-trips through the settings file.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := c.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	c2 := &Config{}
	if err := c2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if c2.AuthTokenValue() != "s3cr3t-token" {
		t.Errorf("token should round-trip via settings file: got %q", c2.AuthTokenValue())
	}

	// Update can replace or clear the token.
	tok2 := "rotated"
	c2.Update(&SettingsUpdate{AuthToken: &tok2})
	if c2.AuthTokenValue() != "rotated" {
		t.Errorf("token rotation failed: %q", c2.AuthTokenValue())
	}
	cleared := ""
	c2.Update(&SettingsUpdate{AuthToken: &cleared})
	if c2.AuthTokenValue() != "" {
		t.Errorf("empty update should clear the token: %q", c2.AuthTokenValue())
	}
}

// The accessors must be safe to call concurrently with Update (handlers read
// config fields on every request while PUT /api/settings mutates them). Run
// with -race to verify.
func TestConfigAccessorsConcurrentWithUpdate(t *testing.T) {
	c := &Config{StorageType: "local", ImageFormat: "avif"}
	accessKey := "ak"
	secretKey := "sk"
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.Update(&SettingsUpdate{
				ImageFormat: ptr("webp"),
				S3AccessKey: &accessKey,
				S3SecretKey: &secretKey,
			})
		}
		close(done)
	}()
	for i := 0; i < 200; i++ {
		_ = c.GetImageFormat()
		_, _ = c.GetStorageMode()
		_ = c.GetS3Settings()
		_ = c.AuthTokenValue()
		_ = c.GetSettingsResponse()
	}
	wg.Wait()
}

func ptr(s string) *string { return &s }

func TestBackupSettingsRoundTrip(t *testing.T) {
	c := &Config{}
	if c.GetBackupIntervalHours() != 0 {
		t.Error("backup should be disabled by default")
	}
	if c.GetBackupKeep() != 10 {
		t.Errorf("default keep should be 10, got %d", c.GetBackupKeep())
	}

	interval := 6
	keep := 3
	c.Update(&SettingsUpdate{BackupIntervalHours: &interval, BackupKeep: &keep})
	if c.GetBackupIntervalHours() != 6 || c.GetBackupKeep() != 3 {
		t.Fatalf("update failed: interval=%d keep=%d", c.GetBackupIntervalHours(), c.GetBackupKeep())
	}
	if r := c.GetSettingsResponse(); r["backup_interval_hours"] != 6 || r["backup_keep"] != 3 {
		t.Errorf("settings response missing backup fields: %v", r)
	}

	// 保存/加载往返
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := c.SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	c2 := &Config{}
	if err := c2.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}
	if c2.GetBackupIntervalHours() != 6 || c2.GetBackupKeep() != 3 {
		t.Fatalf("round trip: interval=%d keep=%d", c2.GetBackupIntervalHours(), c2.GetBackupKeep())
	}

	// 越界值收敛：负间隔归 0（关闭），保留份数下限 1
	neg := -5
	zeroKeep := 0
	c2.Update(&SettingsUpdate{BackupIntervalHours: &neg, BackupKeep: &zeroKeep})
	if c2.GetBackupIntervalHours() != 0 {
		t.Error("negative interval should clamp to 0 (disabled)")
	}
	if c2.GetBackupKeep() != 1 {
		t.Errorf("keep=0 should clamp to minimum 1, got %d", c2.GetBackupKeep())
	}
}

func TestGetAISettingsSnapshot(t *testing.T) {
	c := &Config{}
	c.mu.Lock()
	c.AIEnabled = true
	c.AIBaseURL = "https://api.example.com"
	c.AIAPIKey = "sk-test"
	c.AIModel = "glm-4"
	c.mu.Unlock()

	ai := c.GetAISettings()
	if !ai.Enabled || ai.BaseURL != "https://api.example.com" || ai.APIKey != "sk-test" || ai.Model != "glm-4" {
		t.Fatalf("GetAISettings: %+v", ai)
	}

	// 零值配置返回全部默认空值。
	empty := (&Config{}).GetAISettings()
	if empty.Enabled || empty.BaseURL != "" || empty.APIKey != "" || empty.Model != "" {
		t.Fatalf("empty AISettings: %+v", empty)
	}
}

func TestGetBackupFormatAndRemote(t *testing.T) {
	// 空格式回退 "db"。
	if got := (&Config{}).GetBackupFormat(); got != "db" {
		t.Fatalf("default backup format = %q, want db", got)
	}
	if got := (&Config{BackupFormat: "zip"}).GetBackupFormat(); got != "zip" {
		t.Fatalf("backup format = %q, want zip", got)
	}
	if (&Config{}).GetBackupRemote() {
		t.Fatal("default BackupRemote should be false")
	}
	if !(&Config{BackupRemote: true}).GetBackupRemote() {
		t.Fatal("BackupRemote=true should be reported")
	}
}

func TestGetEnvIntAndB2S(t *testing.T) {
	t.Setenv("MUJIAN_TEST_INT", "42")
	if got := getEnvInt("MUJIAN_TEST_INT", 7); got != 42 {
		t.Fatalf("getEnvInt valid = %d, want 42", got)
	}
	t.Setenv("MUJIAN_TEST_INT", "not-a-number")
	if got := getEnvInt("MUJIAN_TEST_INT", 7); got != 7 {
		t.Fatalf("getEnvInt invalid should fall back, got %d", got)
	}
	if got := getEnvInt("MUJIAN_TEST_UNSET", 7); got != 7 {
		t.Fatalf("getEnvInt unset = %d, want 7", got)
	}
	if b2s(true) != "true" || b2s(false) != "false" {
		t.Fatal("b2s mapping broken")
	}
}
