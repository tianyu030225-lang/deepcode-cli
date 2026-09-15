package main

import (
	"strings"
	"testing"
	"time"
)

func TestToolCallDisplayName(t *testing.T) {
	cases := map[string]string{
		"read_file":    "Read",
		"write_file":   "Write",
		"delete_file":  "Delete",
		"execute_bash": "Bash",
		"get_cwd":      "CWD",
		"unknown":      "unknown",
	}
	for input, want := range cases {
		got := toolCallDisplayName(input)
		if got != want {
			t.Fatalf("toolCallDisplayName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestToolCallArgSummary(t *testing.T) {
	call := ToolCall{
		Function: ToolFunction{
			Name:      "read_file",
			Arguments: `{"path":"/home/user/project/main.go"}`,
		},
	}
	got := toolCallArgSummary(call)
	if got != "main.go" {
		t.Fatalf("toolCallArgSummary = %q, want %q", got, "main.go")
	}
}

func TestConsumeCompletedSubAgentResultsMergesOnce(t *testing.T) {
	app := &App{Session: newSession(defaultDeepSeekConfig())}
	app.Session.BackgroundAgents = []BackgroundAgent{
		{ID: 1, Task: "审查 A", Status: "done", Result: "A ok", StartedAt: time.Now()},
		{ID: 2, Task: "审查 B", Status: "running", StartedAt: time.Now()},
		{ID: 3, Task: "审查 C", Status: "canceled", StartedAt: time.Now()},
	}
	first := app.consumeCompletedSubAgentResults()
	if !strings.Contains(first, "Agent #1 result") || !strings.Contains(first, "A ok") || !strings.Contains(first, "Agent #3 canceled") {
		t.Fatalf("merged result missing: %q", first)
	}
	if !app.Session.BackgroundAgents[0].Merged {
		t.Fatalf("completed agent not marked merged")
	}
	if !app.Session.BackgroundAgents[2].Merged {
		t.Fatalf("canceled agent not marked merged")
	}
	second := app.consumeCompletedSubAgentResults()
	if second != "" {
		t.Fatalf("result merged twice: %q", second)
	}
}

func TestBackgroundAgentStatusContext(t *testing.T) {
	s := newSession(defaultDeepSeekConfig())
	s.BackgroundAgents = []BackgroundAgent{
		{ID: 1, Task: "正在处理用户作业", Status: "running"},
		{ID: 2, Task: "检查测试", Status: "failed"},
	}
	got := backgroundAgentStatusContext(s)
	for _, want := range []string{"子代理真实状态", "Agent #1 running", "Agent #2 failed", "正在处理用户作业"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status context missing %q: %q", want, got)
		}
	}
}
