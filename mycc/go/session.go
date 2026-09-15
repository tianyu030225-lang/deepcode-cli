package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

type Session struct {
	ID                    string            `json:"id"`
	Title                 string            `json:"title"`
	Messages              []Message         `json:"messages"`
	LastReasoning         string            `json:"last_reasoning"`
	Provider              string            `json:"provider"`
	Model                 string            `json:"model"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	ThinkingEnabled       bool              `json:"thinking_enabled"`
	ForceSubAgentNext     bool              `json:"force_sub_agent_next"`
	DangerNext            bool              `json:"danger_next"`
	VeryDangerEnabled     bool              `json:"very_danger_enabled"`
	CompressedSummary     string            `json:"compressed_summary,omitempty"`
	ProviderConfigNote    string            `json:"provider_config_note,omitempty"`
	RestoredNotice        string            `json:"-"`
	TotalPromptTokens     int               `json:"total_prompt_tokens,omitempty"`
	TotalCompletionTokens int               `json:"total_completion_tokens,omitempty"`
	TipsShown             bool              `json:"tips_shown,omitempty"`
	NextBackgroundAgentID int               `json:"next_background_agent_id,omitempty"`
	BackgroundAgents      []BackgroundAgent `json:"background_agents,omitempty"`
}

type BackgroundAgent struct {
	ID                int       `json:"id"`
	Task              string    `json:"task"`
	Status            string    `json:"status"`
	Result            string    `json:"result,omitempty"`
	Error             string    `json:"error,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at,omitempty"`
	DangerNext        bool      `json:"danger_next,omitempty"`
	VeryDangerEnabled bool      `json:"very_danger_enabled,omitempty"`
	Merged            bool      `json:"merged,omitempty"`
	Notified          bool      `json:"notified,omitempty"`
}

type SessionInfo struct {
	ID           string
	Title        string
	Provider     string
	Model        string
	UpdatedAt    time.Time
	Path         string
	MessageCount int
	LastUserText string
}

func newSession(cfg Config) Session {
	now := time.Now()
	return Session{
		ID:        newSessionID(now),
		Title:     "新对话",
		Provider:  cfg.Provider,
		Model:     cfg.DefaultModel,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func sessionPath(paths Paths, id string) string {
	return filepath.Join(paths.SessionDir, id+".json")
}

func saveSession(paths Paths, s *Session) error {
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	toSave := *s
	toSave.DangerNext = false
	toSave.VeryDangerEnabled = false
	if toSave.ID == "" {
		toSave.ID = newSessionID(time.Now())
		s.ID = toSave.ID
	}
	if toSave.Title == "" || toSave.Title == "新对话" {
		toSave.Title = inferSessionTitle(toSave.Messages)
		s.Title = toSave.Title
	}
	toSave.UpdatedAt = time.Now()
	s.UpdatedAt = toSave.UpdatedAt
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(paths, toSave.ID), data, 0o600)
}

func newSessionID(t time.Time) string {
	return fmt.Sprintf("%s-%09d", t.Format("20060102-150405"), t.Nanosecond())
}

func loadSession(paths Paths, id string) (Session, error) {
	data, err := os.ReadFile(sessionPath(paths, id))
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

func listSessions(paths Paths) ([]SessionInfo, error) {
	entries, err := os.ReadDir(paths.SessionDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(paths.SessionDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		out = append(out, SessionInfo{
			ID:           s.ID,
			Title:        fallbackText(s.Title, inferSessionTitle(s.Messages)),
			Provider:     fallbackText(s.Provider, "unknown"),
			Model:        fallbackText(s.Model, "unknown"),
			UpdatedAt:    s.UpdatedAt,
			Path:         path,
			MessageCount: len(s.Messages),
			LastUserText: lastUserText(s.Messages),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func lastUserText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return oneLinePreview(messages[i].Content, 36)
		}
	}
	return ""
}

func lastAssistantText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
			return oneLinePreview(messages[i].Content, 60)
		}
	}
	return ""
}

func oneLinePreview(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	text = strings.Join(strings.Fields(text), " ")
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return text
}

func inferSessionTitle(messages []Message) string {
	for _, msg := range messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			title := strings.TrimSpace(msg.Content)
			title = strings.ReplaceAll(title, "\n", " ")
			if len([]rune(title)) > 24 {
				runes := []rune(title)
				title = string(runes[:24]) + "..."
			}
			return title
		}
	}
	return fmt.Sprintf("对话 %s", time.Now().Format("01-02 15:04"))
}
