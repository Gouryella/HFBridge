package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

const (
	defaultUpstreamURL = "https://huggingface.co"
	defaultAllowHosts  = "huggingface.co,cdn-lfs.huggingface.co,*.huggingface.co,*.hf.co"
	defaultBindAddr    = ":8080"
	defaultDBPath      = "data.db"
)

type Config struct {
	BindAddr        string
	DefaultUpstream string
	ProxyOrigin     string
	HFToken         string
	AllowHosts      []string
	DBPath          string
}

func FromEnv() (Config, error) {
	if err := ensureEnvLoaded(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		BindAddr:        getEnv("BIND_ADDR", defaultBindAddr),
		DefaultUpstream: getEnv("DEFAULT_UPSTREAM", defaultUpstreamURL),
		ProxyOrigin:     os.Getenv("PROXY_ORIGIN"),
		HFToken:         os.Getenv("HF_TOKEN"),
		AllowHosts:      parseAllowHosts(getEnv("ALLOW_HOSTS", defaultAllowHosts)),
		DBPath:          getEnv("DB_PATH", getEnv("LOG_DB_PATH", defaultDBPath)),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseAllowHosts(csv string) []string {
	parts := strings.Split(csv, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var (
	envOnce    sync.Once
	envOnceErr error
)

func ensureEnvLoaded() error {
	envOnce.Do(func() {
		if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
			envOnceErr = err
		}
	})

	if envOnceErr != nil {
		return fmt.Errorf("load .env: %w", envOnceErr)
	}

	return nil
}
