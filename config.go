package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type S3Config struct {
	Credentials string `json:"credentials,omitempty"`
	Region      string `json:"region,omitempty"`
	Bucket      string `json:"bucket"`
}

type Config struct {
	BindTo             string          `json:"bind_to,omitempty"`
	MaxClients         int             `json:"max_clients,omitempty"`
	WorkingQueueSize   int             `json:"working_queue_size,omitempty"`
	Concurrency        int             `json:"concurrency,omitempty"`
	FreeMemoryInterval int             `json:"free_memory_interval,omitempty"`
	RemoteStorage      bool            `json:"remote_storage,omitempty"`
	BasePath           string          `json:"base_path,omitempty"`
	S3                 S3Config        `json:"s3,omitempty"`
	SignatureMethod    string          `json:"signature_method,omitempty"`
	SignatureSecret    string          `json:"signature_secret,omitempty"`
	LogFile            string          `json:"log_file,omitempty"`
	CachePath          string          `json:"cache_path,omitempty"`
	Presets            json.RawMessage `json:"presets,omitempty"`
	WebpQCorrection    int             `json:"webp_q_correction,omitempty"`
	MemoryLimit        int64           `json:"go_memory_limit,omitempty"`
}

var cfg Config

func ParseConfig(configFile string) (*Config, error) {
	cfg = Config{
		BindTo:             "127.0.0.1:8081",
		MaxClients:         200,
		Concurrency:        1,
		WorkingQueueSize:   5,
		FreeMemoryInterval: 20,
		MemoryLimit:        80 * 1024 * 1024,
		CachePath:          "",
		WebpQCorrection:    -2,
	}

	path, _ := filepath.Abs(configFile)

	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %s (%w)", path, err)
	}

	if err := json.Unmarshal(text, &cfg); err != nil {
		return nil, err
	}

	if cfg.S3.Credentials == "" {
		cfg.S3.Credentials = ".aws_credentials"
	} else if _, err := os.Stat(cfg.S3.Credentials); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("aws credentials files doesn't exist (%s)", cfg.S3.Credentials)
	}

	if cfg.S3.Region == "" {
		cfg.S3.Region = "ru-central1"
	}

	if cfg.RemoteStorage && cfg.S3.Bucket == "" {
		return nil, fmt.Errorf("s3.bucket must not be empty")
	}

	if cfg.LogFile != "" {
		var err error
		cfg.LogFile, err = filepath.Abs(cfg.LogFile)
		if err != nil {
			return nil, fmt.Errorf("failed to process log file path: %s (%w)", cfg.LogFile, err)
		}
	}

	return &cfg, nil
}
