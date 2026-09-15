package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	if len(os.Args) > 1 && os.Args[1] == "--sub-agent" {
		subAgentProcess = true
		return runSubAgentMode(os.Args[2:])
	}
	forceShow := false
	settingMode := false
	for _, arg := range os.Args[1:] {
		if arg == "-show" || arg == "--show" {
			forceShow = true
		}
		if arg == "-setting" || arg == "--setting" {
			settingMode = true
		}
	}
	paths, err := userPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !isInstalledBinary(paths) {
		if err := runInstall(paths); err != nil {
			fmt.Fprintln(os.Stderr, "安装失败:", err)
			return 1
		}
		return 0
	}
	cfg, err := loadConfig(paths)
	if settingMode {
		cfg, err = runConfigWizard(paths, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg, err = runConfigWizard(paths, true)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if forceShow || !cfg.StartupShown {
		showStartupAnimation(true)
		cfg.StartupShown = true
		_ = saveConfig(paths, cfg)
	}
	app, err := newApp(paths, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	clearScreen()
	renderWelcomePanel(app.Config, app.Session)
	if err := app.loop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newApp(paths Paths, cfg Config) (*App, error) {
	s := newSession(cfg)
	s.DangerNext = false
	s.VeryDangerEnabled = false
	if err := saveSession(paths, &s); err != nil {
		return nil, err
	}
	cfg.CurrentSessionID = s.ID
	if err := saveConfig(paths, cfg); err != nil {
		return nil, err
	}
	return &App{Paths: paths, Config: cfg, Session: s}, nil
}

func (a *App) loop() error {
	for {
		a.printBackgroundAgentNotifications()
		if a.Session.VeryDangerEnabled {
			dangerLine("[持续危险权限已开启]")
		} else if a.Session.DangerNext {
			dangerLine("[下一次任务危险权限已开启]")
		}
		line, err := readInputLine(a)
		if err != nil {
			return err
		}
		if line == "" {
			continue
		}
		if line[0] == '/' {
			quit, err := a.handleCommand(line)
			if err != nil {
				dangerLine(err.Error())
			}
			if quit {
				return nil
			}
			continue
		}
		if err := a.runUserTurn(line); err != nil {
			dangerLine(err.Error())
		}
	}
}
