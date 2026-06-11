package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLocalStorageConfig(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "images")
	cfg := parseConfigText(t, `{
		"storage": {
			"type": "local",
			"local_path": `+quote(localPath)+`
		}
	}`)

	if cfg.Storage.Type != "local" {
		t.Fatalf("got storage type %q, want local", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != localPath {
		t.Fatalf("got local path %q, want %q", cfg.Storage.LocalPath, localPath)
	}
}

func TestParseOverlayStorageConfig(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "images")
	cachePath := filepath.Join(dir, "cache")
	credentials := filepath.Join(dir, "credentials")
	writeCredentialsFile(t, credentials)

	cfg := parseConfigText(t, `{
		"storage": {
			"type": "overlay",
			"local_path": `+quote(localPath)+`,
			"cache_path": `+quote(cachePath)+`,
			"credentials": `+quote(credentials)+`,
			"bucket": "bucket"
		}
	}`)

	if cfg.Storage.Type != "overlay" {
		t.Fatalf("got storage type %q, want overlay", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != localPath {
		t.Fatalf("got local path %q, want %q", cfg.Storage.LocalPath, localPath)
	}
	if cfg.Storage.CachePath != cachePath {
		t.Fatalf("got cache path %q, want %q", cfg.Storage.CachePath, cachePath)
	}
	if cfg.Storage.Credentials != credentials {
		t.Fatalf("got credentials path %q, want %q", cfg.Storage.Credentials, credentials)
	}
	if cfg.Storage.Region != "ru-central1" {
		t.Fatalf("got region %q, want ru-central1", cfg.Storage.Region)
	}
	if cfg.Storage.Bucket != "bucket" {
		t.Fatalf("got bucket %q, want bucket", cfg.Storage.Bucket)
	}
}

func TestParseWithOverridesChangesOnlyRequestedFields(t *testing.T) {
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials")
	oldCachePath := filepath.Join(dir, "old-cache")
	newCachePath := filepath.Join(dir, "new-cache")
	writeCredentialsFile(t, credentials)

	cfg := parseConfigTextWithOverrides(t, `{
		"resizer": {
			"output_format": "webp"
		},
		"storage": {
			"type": "s3",
			"cache_path": `+quote(oldCachePath)+`,
			"credentials": `+quote(credentials)+`,
			"region": "custom-region",
			"bucket": "bucket",
			"max_width": 1200,
			"max_height": 900
		}
	}`, Overrides{
		StorageCachePath: newCachePath,
	})

	if cfg.Storage.Type != "s3" {
		t.Fatalf("got storage type %q, want s3", cfg.Storage.Type)
	}
	if cfg.Storage.CachePath != newCachePath {
		t.Fatalf("got cache path %q, want %q", cfg.Storage.CachePath, newCachePath)
	}
	if cfg.Storage.Credentials != credentials {
		t.Fatalf("got credentials path %q, want %q", cfg.Storage.Credentials, credentials)
	}
	if cfg.Storage.Region != "custom-region" {
		t.Fatalf("got region %q, want custom-region", cfg.Storage.Region)
	}
	if cfg.Storage.Bucket != "bucket" {
		t.Fatalf("got bucket %q, want bucket", cfg.Storage.Bucket)
	}
	if cfg.Storage.MaxWidth != 1200 || cfg.Storage.MaxHeight != 900 {
		t.Fatalf("got max size %dx%d, want 1200x900", cfg.Storage.MaxWidth, cfg.Storage.MaxHeight)
	}
	if cfg.Resizer.OutputType != OutputTypeWebp {
		t.Fatalf("got output format %q, want webp", cfg.Resizer.OutputType)
	}
}

func TestParseAcceptsOverridesArgument(t *testing.T) {
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials")
	localPath := filepath.Join(dir, "local")
	writeCredentialsFile(t, credentials)

	cfg, err := Parse(writeConfigText(t, `{
		"storage": {
			"type": "s3",
			"credentials": `+quote(credentials)+`,
			"bucket": "bucket",
			"local_path": `+quote(localPath)+`
		}
	}`), Overrides{
		StorageType: "local",
	})
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if cfg.Storage.Type != "local" {
		t.Fatalf("got storage type %q, want local", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != localPath {
		t.Fatalf("got local path %q, want %q", cfg.Storage.LocalPath, localPath)
	}
}

func TestParseWithOverridesCanTurnS3ConfigIntoOverlay(t *testing.T) {
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials")
	localPath := filepath.Join(dir, "local")
	cachePath := filepath.Join(dir, "cache")
	writeCredentialsFile(t, credentials)

	cfg := parseConfigTextWithOverrides(t, `{
		"storage": {
			"type": "s3",
			"credentials": `+quote(credentials)+`,
			"region": "custom-region",
			"bucket": "bucket",
			"max_width": 1200,
			"max_height": 900
		}
	}`, Overrides{
		StorageType:      "overlay",
		StorageLocalPath: localPath,
		StorageCachePath: cachePath,
	})

	if cfg.Storage.Type != "overlay" {
		t.Fatalf("got storage type %q, want overlay", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != localPath {
		t.Fatalf("got local path %q, want %q", cfg.Storage.LocalPath, localPath)
	}
	if cfg.Storage.CachePath != cachePath {
		t.Fatalf("got cache path %q, want %q", cfg.Storage.CachePath, cachePath)
	}
	if cfg.Storage.Region != "custom-region" {
		t.Fatalf("got region %q, want custom-region", cfg.Storage.Region)
	}
	if cfg.Storage.Bucket != "bucket" {
		t.Fatalf("got bucket %q, want bucket", cfg.Storage.Bucket)
	}
	if cfg.Storage.MaxWidth != 1200 || cfg.Storage.MaxHeight != 900 {
		t.Fatalf("got max size %dx%d, want 1200x900", cfg.Storage.MaxWidth, cfg.Storage.MaxHeight)
	}
}

func TestParseRejectsLocalStorageWithoutPath(t *testing.T) {
	_, err := Parse(writeConfigText(t, `{
		"storage": {
			"type": "local"
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "storage.local_path") {
		t.Fatalf("got error %v, want storage.local_path validation", err)
	}
}

func TestParseRejectsUnsupportedStorageType(t *testing.T) {
	_, err := Parse(writeConfigText(t, `{
		"storage": {
			"type": "unknown"
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported storage.type") {
		t.Fatalf("got error %v, want unsupported storage.type validation", err)
	}
}

func TestParseDefaultsStorageTypeToS3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeCredentialsFile(t, filepath.Join(dir, ".aws_credentials"))

	cfg := parseConfigText(t, `{
		"storage": {
			"bucket": "bucket"
		}
	}`)

	if cfg.Storage.Type != "s3" {
		t.Fatalf("got storage type %q, want s3", cfg.Storage.Type)
	}
	if cfg.Storage.Region != "ru-central1" {
		t.Fatalf("got region %q, want ru-central1", cfg.Storage.Region)
	}
	if cfg.Storage.Credentials != filepath.Join(dir, ".aws_credentials") {
		t.Fatalf("got credentials path %q, want absolute default path", cfg.Storage.Credentials)
	}
}

func TestParseRejectsMissingS3Credentials(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing_credentials")

	_, err := Parse(writeConfigText(t, `{
		"storage": {
			"bucket": "bucket",
			"credentials": `+quote(missing)+`
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "storage.credentials") {
		t.Fatalf("got error %v, want storage.credentials validation", err)
	}
}

func TestParseDefaultMaxUploadSize(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "images")
	cfg := parseConfigText(t, `{
		"storage": {
			"type": "local",
			"local_path": `+quote(localPath)+`
		}
	}`)

	if cfg.Server.MaxUploadSize != 64*1024*1024 {
		t.Fatalf("got max upload size %d, want 64 MiB", cfg.Server.MaxUploadSize)
	}
}

func TestParseNormalizesUploadFileBasePath(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "images")
	t.Chdir(dir)

	cfg := parseConfigText(t, `{
		"server": {
			"upload_file_base_path": "uploads"
		},
		"storage": {
			"type": "local",
			"local_path": `+quote(localPath)+`
		}
	}`)

	if cfg.Server.UploadFileBasePath != filepath.Join(dir, "uploads") {
		t.Fatalf("got upload file base path %q, want absolute path", cfg.Server.UploadFileBasePath)
	}
}

func parseConfigText(t *testing.T, text string) *Config {
	t.Helper()

	cfg, err := Parse(writeConfigText(t, text))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	return cfg
}

func parseConfigTextWithOverrides(t *testing.T, text string, overrides Overrides) *Config {
	t.Helper()

	cfg, err := ParseWithOverrides(writeConfigText(t, text), overrides)
	if err != nil {
		t.Fatalf("ParseWithOverrides returned an error: %v", err)
	}
	return cfg
}

func writeConfigText(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func writeCredentialsFile(t *testing.T, path string) {
	t.Helper()

	contents := []byte("[default]\naws_access_key_id = test-key\naws_secret_access_key = test-secret\n")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatalf("failed to write credentials: %v", err)
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
