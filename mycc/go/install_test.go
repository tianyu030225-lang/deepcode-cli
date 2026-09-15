package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLocalBinInShellPATHWritesPreferredShellOnly(t *testing.T) {
	home := t.TempDir()
	paths := Paths{Home: home, BinDir: filepath.Join(home, ".local", "bin")}
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SHELL", "/bin/zsh")

	updated, err := ensureLocalBinInShellPATH(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected PATH update")
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("expected .zshrc update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .bashrc update: %v", err)
	}
}

func TestEnsureLocalBinInShellPATHSkipsExistingShellConfig(t *testing.T) {
	home := t.TempDir()
	paths := Paths{Home: home, BinDir: filepath.Join(home, ".local", "bin")}
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SHELL", "/bin/zsh")

	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(`export PATH="$HOME/.local/bin:$PATH"`), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := ensureLocalBinInShellPATH(paths)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("expected no update when an existing shell config already has .local/bin")
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .zshrc update: %v", err)
	}
}

func TestPathContainsUsesExactPathEntry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	t.Setenv("PATH", dir+"suffix"+string(os.PathListSeparator)+"/usr/bin")
	if pathContains(dir) {
		t.Fatal("partial PATH entry should not match")
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin")
	if !pathContains(dir) {
		t.Fatal("exact PATH entry should match")
	}
}

func TestPreferredShellPathFileFallsBackToProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/usr/bin/fish")
	got := preferredShellPathFile(Paths{Home: home})
	if !strings.HasSuffix(got, ".profile") {
		t.Fatalf("preferred file = %q", got)
	}
}
