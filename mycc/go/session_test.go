package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestListSessionsIncludesResumeMetadata(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: filepath.Join(dir, "config.json"), SessionDir: filepath.Join(dir, "sessions"), BinDir: filepath.Join(dir, "bin")}
	cfg := defaultDeepSeekConfig()
	s := newSession(cfg)
	s.Messages = append(s.Messages,
		Message{Role: "user", Content: "第一题怎么做？"},
		Message{Role: "assistant", Content: "先读取作业文件。"},
		Message{Role: "user", Content: "继续完成代码"},
	)
	if err := saveSession(paths, &s); err != nil {
		t.Fatal(err)
	}
	sessions, err := listSessions(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count = %d", len(sessions))
	}
	info := sessions[0]
	if info.Model != cfg.DefaultModel || info.Provider != cfg.Provider {
		t.Fatalf("metadata missing: %#v", info)
	}
	if info.MessageCount != len(s.Messages) {
		t.Fatalf("message count = %d", info.MessageCount)
	}
	if !strings.Contains(info.LastUserText, "继续完成代码") {
		t.Fatalf("last user preview = %q", info.LastUserText)
	}
}

func TestBuildMessagesIncludesRestoredNotice(t *testing.T) {
	cfg := defaultDeepSeekConfig()
	s := newSession(cfg)
	s.RestoredNotice = "当前 CLI 刚刚恢复了一个本地历史对话。"
	s.Messages = append(s.Messages, Message{Role: "user", Content: "历史问题"})
	messages := buildMessages(cfg, s, PermissionState{})
	if len(messages) < 3 {
		t.Fatalf("messages too short: %#v", messages)
	}
	if !strings.Contains(messages[1].Content, "恢复") {
		t.Fatalf("restored notice missing: %#v", messages)
	}
	if messages[2].Content != "历史问题" {
		t.Fatalf("history position changed: %#v", messages)
	}
}
