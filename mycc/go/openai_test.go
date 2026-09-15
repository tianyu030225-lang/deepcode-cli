package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSSEContentReasoningAndTool(t *testing.T) {
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"想一下","content":""}}]}`,
		`data: {"choices":[{"delta":{"content":"你好"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\"}"}}]}}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	var content, reasoning string
	var toolStarted string
	var toolArgRunes int
	got, err := parseSSE(context.Background(), strings.NewReader(input), streamCallbacks{
		OnContent:   func(s string) { content += s },
		OnReasoning: func(s string) { reasoning += s },
		OnToolStart: func(s string) { toolStarted = s },
		OnToolDelta: func(_ string, n int) { toolArgRunes += n },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "你好" || content != "你好" {
		t.Fatalf("content = %q callback=%q", got.Content, content)
	}
	if got.ReasoningContent != "想一下" || reasoning != "想一下" {
		t.Fatalf("reasoning = %q callback=%q", got.ReasoningContent, reasoning)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Function.Name != "read_file" || got.ToolCalls[0].Function.Arguments != `{"path":"a.txt"}` {
		t.Fatalf("tool calls = %#v", got.ToolCalls)
	}
	if toolStarted != "read_file" {
		t.Fatalf("tool start = %q", toolStarted)
	}
	if toolArgRunes == 0 {
		t.Fatalf("expected tool argument progress")
	}
}

func TestBuildChatRequestJSONUsesNativeCJSON(t *testing.T) {
	body, err := buildChatRequestJSON("deepseek-test", []Message{{Role: "user", Content: "你好"}}, toolSpecs()[:1], true)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{`"model":"deepseek-test"`, `"messages"`, `"tools"`, `"reasoning_effort":"high"`, `"thinking"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("request body missing %q: %s", want, text)
		}
	}
}

func TestMessageContentAlwaysSerialized(t *testing.T) {
	data, err := json.Marshal(Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Type: "function"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"content":""`) {
		t.Fatalf("content field missing: %s", data)
	}
}

func TestCleanTerminalMarkdown(t *testing.T) {
	r := newReplyRenderer(Config{DisplayMode: displayPlain})
	got := r.cleanMarkdown("### 标题\n这是 **重点**")
	if strings.Contains(got, "###") || strings.Contains(got, "**") {
		t.Fatalf("markdown not cleaned: %q", got)
	}
	if !strings.Contains(got, "▌ 标题") || !strings.Contains(got, "重点") {
		t.Fatalf("unexpected clean output: %q", got)
	}
}

func TestStreamingReplyRendererQueuesRunesFromChunks(t *testing.T) {
	r := &streamingReplyRenderer{base: newReplyRenderer(Config{DisplayMode: displayPlain})}
	r.Write("你好")
	r.Write(" world")

	for _, want := range []string{"你", "好", " ", "w"} {
		got, _, ok := r.nextUnitLocked()
		if !ok || got != want {
			t.Fatalf("next unit = %q ok=%v, want %q", got, ok, want)
		}
	}
}
