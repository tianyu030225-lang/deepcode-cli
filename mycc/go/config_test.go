package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNormalizeConfigDefaults(t *testing.T) {
	var cfg Config
	normalizeConfig(&cfg)
	if cfg.Provider != providerDeepSeek {
		t.Fatalf("provider = %q", cfg.Provider)
	}
	if cfg.BaseURL != defaultBaseURL || cfg.DefaultModel != defaultModel || cfg.LightModel != defaultLightModel || cfg.StrongModel != defaultStrongModel {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
	if cfg.ReplyStyle != "适中" {
		t.Fatalf("reply style = %q", cfg.ReplyStyle)
	}
	if cfg.DisplayMode != displayColorful {
		t.Fatalf("display mode = %q", cfg.DisplayMode)
	}
}

func TestSaveConfigClearsVolatileFlags(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: dir + "/config.json", SessionDir: dir + "/sessions", BinDir: dir + "/bin"}
	cfg := defaultDeepSeekConfig()
	cfg.ThinkingEnabled = true
	cfg.VeryDangerEnabled = true
	if err := saveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ThinkingEnabled || saved.VeryDangerEnabled {
		t.Fatalf("volatile flags persisted: %#v", saved)
	}
}

func TestNewAppStartsFreshSession(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: dir + "/config.json", SessionDir: dir + "/sessions", BinDir: dir + "/bin"}
	cfg := defaultDeepSeekConfig()
	old := newSession(cfg)
	old.Messages = append(old.Messages, Message{Role: "user", Content: "old message"})
	if err := saveSession(paths, &old); err != nil {
		t.Fatal(err)
	}
	cfg.CurrentSessionID = old.ID
	app, err := newApp(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if app.Session.ID == old.ID {
		t.Fatalf("expected fresh session, got old session %s", old.ID)
	}
	if len(app.Session.Messages) != 0 {
		t.Fatalf("fresh session should not include history: %#v", app.Session.Messages)
	}
}

func TestClearLocalConfigRemovesConfigDir(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: dir + "/config.json", SessionDir: dir + "/sessions", BinDir: dir + "/bin"}
	if err := os.WriteFile(paths.ConfigFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clearLocalConfig(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.ConfigFile); !os.IsNotExist(err) {
		t.Fatalf("config still exists or stat failed unexpectedly: %v", err)
	}
}
