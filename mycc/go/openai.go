package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatRequest struct {
	Model           string      `json:"model"`
	Messages        []Message   `json:"messages"`
	Stream          bool        `json:"stream"`
	Tools           []ToolSpec  `json:"tools,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	Thinking        interface{} `json:"thinking,omitempty"`
}

type ToolSpec struct {
	Type     string           `json:"type"`
	Function ToolSpecFunction `json:"function"`
}

type ToolSpecFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			ToolCalls        []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		Message *Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Usage *TokenUsage `json:"usage"`
}

type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type assistantStream struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	Usage            TokenUsage
}

type streamCallbacks struct {
	OnContent   func(string)
	OnReasoning func(string)
	OnToolStart func(string)
	OnToolDelta func(string, int)
}

func chatStream(ctx context.Context, cfg Config, model string, messages []Message, tools []ToolSpec, thinking bool, callbacks streamCallbacks) (assistantStream, error) {
	normalizeConfig(&cfg)
	if model == "" {
		model = cfg.DefaultModel
	}
	body, err := buildChatRequestJSON(model, messages, tools, thinking)
	if err != nil {
		return assistantStream{}, err
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return assistantStream{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return assistantStream{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return assistantStream{}, fmt.Errorf("API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = resp.Body.Close()
		case <-done:
		}
	}()
	stream, err := parseSSE(ctx, resp.Body, callbacks)
	close(done)
	if err != nil && ctx.Err() != nil {
		return stream, ctx.Err()
	}
	return stream, err
}

func buildChatRequestJSON(model string, messages []Message, tools []ToolSpec, thinking bool) ([]byte, error) {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	toolsJSON := ""
	if len(tools) > 0 {
		data, err := json.Marshal(tools)
		if err != nil {
			return nil, err
		}
		toolsJSON = string(data)
	}
	r := nativeBuildChatRequestJSON(model, string(messagesJSON), toolsJSON, thinking)
	if err := r.err(); err != nil {
		return nil, err
	}
	return []byte(r.Output), nil
}

type parsedSSEChunk struct {
	Error            string          `json:"error"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        []toolCallDelta `json:"tool_calls"`
	Usage            *TokenUsage     `json:"usage"`
}

func parseSSE(ctx context.Context, r io.Reader, callbacks streamCallbacks) (assistantStream, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var result assistantStream
	toolBuilders := map[int]*ToolCall{}
	toolAnnounced := map[int]bool{}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		parsed, err := parseSSEChunkJSON(data)
		if err != nil {
			return result, err
		}
		if parsed.Error != "" {
			return result, fmt.Errorf("API error: %s", parsed.Error)
		}
		if parsed.Usage != nil {
			result.Usage = *parsed.Usage
		}
		if parsed.ReasoningContent != "" {
			result.ReasoningContent += parsed.ReasoningContent
			if callbacks.OnReasoning != nil {
				callbacks.OnReasoning(parsed.ReasoningContent)
			}
		}
		if parsed.Content != "" {
			result.Content += parsed.Content
			if callbacks.OnContent != nil {
				callbacks.OnContent(parsed.Content)
			}
		}
		for _, tc := range parsed.ToolCalls {
			builder := toolBuilders[tc.Index]
			if builder == nil {
				builder = &ToolCall{Type: "function"}
				toolBuilders[tc.Index] = builder
			}
			if tc.ID != "" {
				builder.ID = tc.ID
			}
			if tc.Type != "" {
				builder.Type = tc.Type
			}
			if tc.Function.Name != "" {
				builder.Function.Name = tc.Function.Name
				if callbacks.OnToolStart != nil && !toolAnnounced[tc.Index] {
					toolAnnounced[tc.Index] = true
					callbacks.OnToolStart(tc.Function.Name)
				}
			}
			if tc.Function.Arguments != "" {
				builder.Function.Arguments += tc.Function.Arguments
				if callbacks.OnToolDelta != nil {
					callbacks.OnToolDelta(builder.Function.Name, len([]rune(tc.Function.Arguments)))
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	for i := 0; ; i++ {
		tc := toolBuilders[i]
		if tc == nil {
			break
		}
		result.ToolCalls = append(result.ToolCalls, *tc)
	}
	return result, nil
}

func parseSSEChunkJSON(data string) (parsedSSEChunk, error) {
	r := nativeParseSSEChunkJSON(data)
	if err := r.err(); err != nil {
		return parsedSSEChunk{}, fmt.Errorf("解析流式响应失败: %w", err)
	}
	var parsed parsedSSEChunk
	if err := json.Unmarshal([]byte(r.Output), &parsed); err != nil {
		return parsedSSEChunk{}, fmt.Errorf("解析流式响应失败: %w", err)
	}
	return parsed, nil
}

func quickChat(cfg Config, model string, messages []Message, thinking bool) (assistantStream, error) {
	return quickChatWithCallbacks(cfg, model, messages, thinking, streamCallbacks{})
}

func quickChatWithCallbacks(cfg Config, model string, messages []Message, thinking bool, callbacks streamCallbacks) (assistantStream, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return chatStream(ctx, cfg, model, messages, nil, thinking, callbacks)
}
