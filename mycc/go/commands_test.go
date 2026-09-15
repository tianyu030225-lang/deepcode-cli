package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalReviewCommands(t *testing.T) {
	if canonicalCommand("/review") != "/review" {
		t.Fatalf("/review canonical mismatch")
	}
	if canonicalCommand("/审查") != "/review" {
		t.Fatalf("/审查 canonical mismatch")
	}
}

func TestSplitSlashCommandSupportsRenameArgument(t *testing.T) {
	name, arg := splitSlashCommand("/rename 新会话")
	if name != "/rename" || arg != "新会话" {
		t.Fatalf("split = %q %q", name, arg)
	}
}

func TestRenameSessionUpdatesTitle(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir, ConfigFile: filepath.Join(dir, "config.json"), SessionDir: filepath.Join(dir, "sessions"), BinDir: filepath.Join(dir, "bin")}
	app := &App{Paths: paths, Config: defaultDeepSeekConfig(), Session: newSession(defaultDeepSeekConfig())}
	if err := app.renameSession("项目审查"); err != nil {
		t.Fatal(err)
	}
	if app.Session.Title != "项目审查" {
		t.Fatalf("title = %q", app.Session.Title)
	}
	sessions, err := listSessions(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Title != "项目审查" {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestCodeReviewCommandPromptIncludesSnapshot(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build", "generated.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	prompt, err := codeReviewCommandPrompt()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"请审查当前工作目录的所有代码", "审查目标", "--- main.go ---", "func main()"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "generated.go ---") {
		t.Fatalf("build output should not be included in review snapshot:\n%s", prompt)
	}
}
