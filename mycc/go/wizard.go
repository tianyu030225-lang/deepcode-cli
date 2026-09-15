package main

import (
	"fmt"
	"strings"
)

func runConfigWizard(paths Paths, forceAnimation bool) (Config, error) {
	showStartupAnimation(forceAnimation)
	cfg := defaultDeepSeekConfig()
	choice, err := chooseMenu("选择 API provider", []string{"DeepSeek API", "自定义 API"})
	if err != nil {
		return Config{}, err
	}
	if choice == 0 {
		cfg.Provider = providerDeepSeek
		cfg.BaseURL = defaultBaseURL
		cfg.DefaultModel = defaultModel
		cfg.LightModel = defaultLightModel
		cfg.StrongModel = defaultStrongModel
		key, err := promptLine("填入 DeepSeek API Key: ", true)
		if err != nil {
			return Config{}, err
		}
		cfg.APIKey = key
	} else {
		cfg.Provider = providerCustom
		var err error
		if cfg.APIKey, err = promptLine("填入 API Key: ", true); err != nil {
			return Config{}, err
		}
		if cfg.BaseURL, err = promptLine("填入 Base URL: ", false); err != nil {
			return Config{}, err
		}
		if cfg.DefaultModel, err = promptLine("默认模型: ", false); err != nil {
			return Config{}, err
		}
		if cfg.LightModel, err = promptLine("轻量模型: ", false); err != nil {
			return Config{}, err
		}
		if cfg.StrongModel, err = promptLine("最强模型: ", false); err != nil {
			return Config{}, err
		}
	}
	style, err := chooseMenu("选择回复风格", []string{"简洁优先", "适中", "详细"})
	if err != nil {
		return Config{}, err
	}
	cfg.ReplyStyle = []string{"简洁优先", "适中", "详细"}[style]
	display, err := chooseMenu("选择显示模式", []string{displayColorful, displayPlain})
	if err != nil {
		return Config{}, err
	}
	cfg.DisplayMode = []string{displayColorful, displayPlain}[display]
	cfg.StartupShown = true
	normalizeConfig(&cfg)
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Config{}, fmt.Errorf("API key 不能为空")
	}
	if err := saveConfig(paths, cfg); err != nil {
		return Config{}, err
	}
	clearScreen()
	return cfg, nil
}
