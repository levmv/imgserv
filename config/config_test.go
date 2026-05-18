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

func parseConfigText(t *testing.T, text string) *Config {
	t.Helper()

	cfg, err := Parse(writeConfigText(t, text))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
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
