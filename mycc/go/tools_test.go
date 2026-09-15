package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseToolArgsRejectsWorkspaceTraversal(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	_, errText := parseToolArgs(ToolCall{Function: ToolFunction{Name: "read_file", Arguments: `{"path":"../outside.txt"}`}})
	if !strings.Contains(errText, "超出当前工作目录") {
		t.Fatalf("expected traversal rejection, got %q", errText)
	}
}

func TestParseToolArgsAcceptsWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	args, errText := parseToolArgs(ToolCall{Function: ToolFunction{Name: "write_file", Arguments: `{"path":"sub/file.txt","content":""}`}})
	if errText != "" {
		t.Fatalf("unexpected error: %q", errText)
	}
	want := filepath.Join(dir, "sub", "file.txt")
	if args["path"] != want {
		t.Fatalf("path = %q, want %q", args["path"], want)
	}
	if args["content"] != "" {
		t.Fatalf("content = %q, want empty", args["content"])
	}
}

func TestParseToolArgsRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), "link.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, "outside-dir"); err != nil {
		t.Fatal(err)
	}

	calls := []ToolCall{
		{Function: ToolFunction{Name: "read_file", Arguments: `{"path":"link.txt"}`}},
		{Function: ToolFunction{Name: "write_file", Arguments: `{"path":"link.txt","content":"x"}`}},
		{Function: ToolFunction{Name: "delete_file", Arguments: `{"path":"link.txt"}`}},
		{Function: ToolFunction{Name: "write_file", Arguments: `{"path":"outside-dir/new.txt","content":"x"}`}},
	}
	for _, call := range calls {
		if _, errText := parseToolArgs(call); !strings.Contains(errText, "超出当前工作目录") {
			t.Fatalf("expected symlink escape rejection for %s, got %q", call.Function.Name, errText)
		}
	}
}

func TestParseToolArgsRequiresTypedFields(t *testing.T) {
	cases := []ToolCall{
		{Function: ToolFunction{Name: "read_file", Arguments: `{"path":123}`}},
		{Function: ToolFunction{Name: "write_file", Arguments: `{"path":"a.txt"}`}},
		{Function: ToolFunction{Name: "execute_bash", Arguments: `{"command":"   "}`}},
	}
	for _, call := range cases {
		if _, errText := parseToolArgs(call); errText == "" {
			t.Fatalf("expected parse error for %#v", call.Function)
		}
	}
}

func TestNativeCJSONToolArgParserAllowsEmptyWriteContent(t *testing.T) {
	args, errText := parseToolArgs(ToolCall{Function: ToolFunction{Name: "write_file", Arguments: `{"path":"a.txt","content":""}`}})
	if errText != "" {
		t.Fatalf("unexpected error: %q", errText)
	}
	if args["content"] != "" {
		t.Fatalf("content = %q, want empty", args["content"])
	}
}
