package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	providerDeepSeek = "deepseek"
	providerCustom   = "custom"

	displayColorful = "炫彩模式"
	displayPlain    = "简洁模式"

	defaultBaseURL     = "https://api.deepseek.com"
	defaultModel       = "deepseek-v4-pro"
	defaultLightModel  = "deepseek-v4-flash"
	defaultStrongModel = "deepseek-v4-pro"
)

type Config struct {
	Provider          string    `json:"provider"`
	APIKey            string    `json:"api_key"`
	BaseURL           string    `json:"base_url"`
	DefaultModel      string    `json:"default_model"`
	LightModel        string    `json:"light_model"`
	StrongModel       string    `json:"strong_model"`
	ReplyStyle        string    `json:"reply_style"`
	DisplayMode       string    `json:"display_mode"`
	StartupShown      bool      `json:"startup_shown"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CurrentSessionID  string    `json:"current_session_id"`
	ThinkingEnabled   bool      `json:"thinking_enabled"`
	VeryDangerEnabled bool      `json:"very_danger_enabled"`
}

type Paths struct {
	Home       string
	ConfigDir  string
	ConfigFile string
	SessionDir string
	BinDir     string
	BinPath    string
}

func userPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	configDir := filepath.Join(home, ".deepcode")
	return Paths{
		Home:       home,
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, "config.json"),
		SessionDir: filepath.Join(configDir, "sessions"),
		BinDir:     filepath.Join(home, ".local", "bin"),
		BinPath:    filepath.Join(home, ".local", "bin", "deepcode"),
	}, nil
}

func ensureDataDirs(paths Paths) error {
	for _, dir := range []string{paths.ConfigDir, paths.SessionDir, paths.BinDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func clearLocalConfig(paths Paths) error {
	return os.RemoveAll(paths.ConfigDir)
}

func loadConfig(paths Paths) (Config, error) {
	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("读取配置失败: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置失败: %w", err)
	}
	normalizeConfig(&cfg)
	return cfg, nil
}

func saveConfig(paths Paths, cfg Config) error {
	normalizeConfig(&cfg)
	cfg.ThinkingEnabled = false
	cfg.VeryDangerEnabled = false
	cfg.UpdatedAt = time.Now()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = cfg.UpdatedAt
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile, data, 0o600)
}

func normalizeConfig(cfg *Config) {
	if cfg.Provider == "" {
		cfg.Provider = providerDeepSeek
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = defaultModel
	}
	if cfg.LightModel == "" {
		cfg.LightModel = defaultLightModel
	}
	if cfg.StrongModel == "" {
		cfg.StrongModel = defaultStrongModel
	}
	if cfg.ReplyStyle == "" {
		cfg.ReplyStyle = "适中"
	}
	if cfg.DisplayMode == "" {
		cfg.DisplayMode = displayColorful
	}
}

func defaultDeepSeekConfig() Config {
	now := time.Now()
	return Config{
		Provider:     providerDeepSeek,
		BaseURL:      defaultBaseURL,
		DefaultModel: defaultModel,
		LightModel:   defaultLightModel,
		StrongModel:  defaultStrongModel,
		ReplyStyle:   "适中",
		DisplayMode:  displayColorful,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
