package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeFileAndBash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if r := nativeWriteFile(path, "hello"); r.Code != 0 {
		t.Fatalf("write failed: %#v", r)
	}
	if r := nativeReadFile(path); r.Code != 0 || r.Output != "hello" {
		t.Fatalf("read = %#v", r)
	}
	if r := nativeExecuteBash("printf native-ok"); r.Code != 0 || strings.TrimSpace(r.Output) != "native-ok" {
		t.Fatalf("bash = %#v", r)
	}
	if r := nativeDeleteFile(path); r.Code != 0 {
		t.Fatalf("delete = %#v", r)
	}
}

func TestNativeExecuteBashDoesNotLoadLoginProfile(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "profile-loaded")
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte("printf loaded > \"$HOME/profile-loaded\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := nativeExecuteBash("printf native-ok"); r.Code != 0 || strings.TrimSpace(r.Output) != "native-ok" {
		t.Fatalf("bash = %#v", r)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("login profile was loaded, marker err=%v", err)
	}
}

func TestSaveSessionClearsVolatilePermissionsOnDisk(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: filepath.Join(dir, "config.json"), SessionDir: filepath.Join(dir, "sessions"), BinDir: filepath.Join(dir, "bin")}
	s := newSession(defaultDeepSeekConfig())
	s.DangerNext = true
	s.VeryDangerEnabled = true
	if err := saveSession(paths, &s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath(paths, s.ID))
	if err != nil {
		t.Fatal(err)
	}
	var saved Session
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DangerNext || saved.VeryDangerEnabled {
		t.Fatalf("volatile permissions persisted: %#v", saved)
	}
	if !s.DangerNext || !s.VeryDangerEnabled {
		t.Fatalf("runtime permissions should remain in memory: %#v", s)
	}
}

func TestToolPermission(t *testing.T) {
	if got := runTool(ToolCall{Function: ToolFunction{Name: "get_cwd", Arguments: `{}`}}, PermissionState{}); got == "" || strings.Contains(got, "failed") {
		t.Fatalf("get_cwd failed: %q", got)
	}
	call := ToolCall{Function: ToolFunction{Name: "execute_bash", Arguments: `{"command":"printf denied"}`}}
	if got := runTool(call, PermissionState{}); !strings.Contains(got, "denied") {
		t.Fatalf("expected denied, got %q", got)
	}
	if got := runTool(call, PermissionState{DangerNext: true}); strings.TrimSpace(got) != "denied" {
		t.Fatalf("expected command output, got %q", got)
	}
}
