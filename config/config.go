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
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	CachePath   string `json:"cache_path"`
	MaxWidth    int    `json:"max_width"`
	MaxHeight   int    `json:"max_height"`
}

type ResizerConf struct {
	SignatureMethod string          `json:"signature_method"`
	SignatureSecret string          `json:"signature_secret"`
	Presets         json.RawMessage `json:"presets"`
	WebpQCorrection int             `json:"webp_q_correction"`
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

	if cfg.Storage.Credentials == "" {
		cfg.Storage.Credentials = ".aws_credentials"
	} else if _, err := os.Stat(cfg.Storage.Credentials); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("aws credentials files doesn't exist (%s)", cfg.Storage.Credentials)
	}

	if cfg.Storage.Region == "" {
		cfg.Storage.Region = "ru-central1"
	}

	if cfg.Storage.Bucket == "" {
		return nil, fmt.Errorf("storage.bucket must not be empty")
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
