package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ServerConf struct {
	BindTo             string `json:"bind_to"`
	MaxClients         int    `json:"max_clients"`
	Concurrency        int    `json:"concurrency"`
	FreeMemoryInterval int    `json:"free_memory_interval"`
	LogFile            string `json:"log_file"`
	MemoryLimit        int64  `json:"go_memory_limit"`
}

type StorageConf struct {
	Credentials string `json:"credentials"`
	Type        string `json:"type"`
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	CachePath   string `json:"cache_path"`
	LocalPath   string `json:"local_path"`
	MaxWidth    int    `json:"max_width"`
	MaxHeight   int    `json:"max_height"`
}

type OutputFormat string

const OutputTypeVary OutputFormat = "vary"
const OutputTypeJpeg OutputFormat = "jpeg"
const OutputTypeWebp OutputFormat = "webp"

type ResizerConf struct {
	SignatureMethod string          `json:"signature_method"`
	SignatureSecret string          `json:"signature_secret"`
	Presets         json.RawMessage `json:"presets"`
	OutputType      OutputFormat    `json:"output_format"`
	WebpQCorrection int             `json:"webp_q_correction"`
	JpegQCorrection int             `json:"jpeg_q_correction"`
}

type SharerConf struct {
	Logo     string `json:"logo"`
	Font     string `json:"font"`
	FontFile string `json:"font_file"`
}

type Config struct {
	Server  ServerConf  `json:"server"`
	Resizer ResizerConf `json:"resizer"`
	Sharer  *SharerConf `json:"sharer"`
	Storage StorageConf `json:"storage"`
}

func Parse(configFile string) (*Config, error) {
	cfg := Config{
		Server: ServerConf{
			BindTo:             "127.0.0.1:8081",
			MaxClients:         100,
			Concurrency:        2,
			FreeMemoryInterval: 20,
			MemoryLimit:        80 * 1024 * 1024,
		},
		Resizer: ResizerConf{
			WebpQCorrection: -2,
			JpegQCorrection: 0,
			OutputType:      OutputTypeVary,
		},
	}

	path, _ := filepath.Abs(configFile)

	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %s (%w)", path, err)
	}

	if err := json.Unmarshal(text, &cfg); err != nil {
		return nil, err
	}

	if cfg.Storage.Type == "" {
		cfg.Storage.Type = "s3"
	}

	switch cfg.Storage.Type {
	case "s3":
		if err := prepareS3Storage(&cfg.Storage); err != nil {
			return nil, err
		}
	case "local":
		if err := prepareLocalStorage(&cfg.Storage); err != nil {
			return nil, err
		}
	case "overlay":
		if err := prepareLocalStorage(&cfg.Storage); err != nil {
			return nil, err
		}
		if err := prepareS3Storage(&cfg.Storage); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported storage.type %q", cfg.Storage.Type)
	}

	if cfg.Server.LogFile != "" {
		var err error
		cfg.Server.LogFile, err = filepath.Abs(cfg.Server.LogFile)
		if err != nil {
			return nil, fmt.Errorf("failed to process log file path: %s (%w)", cfg.Server.LogFile, err)
		}
	}

	return &cfg, nil
}

func prepareS3Storage(conf *StorageConf) error {
	if conf.Credentials == "" {
		conf.Credentials = ".aws_credentials"
	}
	var err error
	conf.Credentials, err = filepath.Abs(conf.Credentials)
	if err != nil {
		return fmt.Errorf("failed to process storage.credentials path: %s (%w)", conf.Credentials, err)
	}
	if err := validateReadableFile(conf.Credentials, "storage.credentials"); err != nil {
		return err
	}

	if conf.Region == "" {
		conf.Region = "ru-central1"
	}

	if conf.Bucket == "" {
		return fmt.Errorf("storage.bucket must not be empty")
	}
	return nil
}

func prepareLocalStorage(conf *StorageConf) error {
	if conf.LocalPath == "" {
		return fmt.Errorf("storage.local_path must not be empty")
	}
	var err error
	conf.LocalPath, err = filepath.Abs(conf.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to process local storage path: %s (%w)", conf.LocalPath, err)
	}
	return nil
}

func validateReadableFile(path string, name string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s file doesn't exist: %s", name, path)
		}
		return fmt.Errorf("failed to read %s file: %s (%w)", name, path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s file: %s (%w)", name, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s must be a file, got directory: %s", name, path)
	}

	return nil
}
