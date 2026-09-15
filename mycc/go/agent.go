package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type App struct {
	Paths   Paths
	Config  Config
	Session Session
	mu      sync.Mutex
}

func (a *App) runUserTurn(input string) error {
	perms := PermissionState{DangerNext: a.Session.DangerNext, VeryDangerEnabled: a.Session.VeryDangerEnabled}
	if a.Session.ForceSubAgentNext {
		a.Session.ForceSubAgentNext = false
		err := a.runSubAgentTurn(input, perms)
		if err == nil {
			a.showFirstTurnTips()
		}
		a.Session.DangerNext = false
		if saveErr := saveSession(a.Paths, &a.Session); saveErr != nil && err == nil {
			err = saveErr
		}
		if err != nil {
			return err
		}
		return nil
	}
	if merged := a.consumeCompletedSubAgentResults(); merged != "" {
		a.Session.Messages = append(a.Session.Messages, Message{Role: "user", Content: merged})
	}
	a.Session.Messages = append(a.Session.Messages, Message{Role: "user", Content: input})
	err := a.completeWithTools(perms)
	if err == nil {
		a.showFirstTurnTips()
	}
	a.Session.DangerNext = false
	if saveErr := saveSession(a.Paths, &a.Session); saveErr != nil && err == nil {
		err = saveErr
	}
	return err
}

func (a *App) completeWithTools(perms PermissionState) error {
	for step := 0; step < 24; step++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		anim := startThinkingAnimation()
		var toolAnim *thinkingAnimation
		var streamRenderer *streamingReplyRenderer
		esc := startEscWatcher(cancel)
		contentStarted := false
		stopActivity := func() {
			if anim != nil {
				anim.Stop()
				anim = nil
			}
			if toolAnim != nil {
				toolAnim.Stop()
				toolAnim = nil
			}
		}
		stopStream := func() {
			if streamRenderer != nil {
				streamRenderer.Stop()
				streamRenderer = nil
			}
		}
		stream, err := chatStream(ctx, a.Config, a.Session.Model, buildMessages(a.Config, a.Session, perms), toolSpecs(), a.Session.ThinkingEnabled, streamCallbacks{
			OnContent: func(text string) {
				if !contentStarted {
					stopActivity()
					streamRenderer = startStreamingReplyRenderer(a.Config)
					contentStarted = true
				}
				streamRenderer.Write(text)
			},
			OnReasoning: func(text string) {
				if anim != nil {
					anim.AddHiddenRunes(len([]rune(text)))
				}
			},
			OnToolStart: func(name string) {
				if anim != nil {
					anim.Stop()
					anim = nil
				}
				stopStream()
				if contentStarted {
					fmt.Println()
				}
				if toolAnim != nil {
					toolAnim.Stop()
				}
				toolAnim = startToolPrepareAnimation(name)
			},
			OnToolDelta: func(name string, n int) {
				if toolAnim != nil {
					toolAnim.AddHiddenRunes(n)
				} else if anim != nil {
					anim.AddHiddenRunes(n)
				}
			},
		})
		stopActivity()
		stopStream()
		wasCanceled := esc.WasCanceled()
		if err != nil {
			esc.Stop()
			cancel()
			if wasCanceled {
				return fmt.Errorf("已按 Esc 取消本次请求")
			}
			return err
		}
		if wasCanceled {
			esc.Stop()
			cancel()
			return fmt.Errorf("已按 Esc 取消本次请求")
		}
		esc.Stop()
		fmt.Println()
		a.Session.TotalPromptTokens += stream.Usage.PromptTokens
		a.Session.TotalCompletionTokens += stream.Usage.CompletionTokens
		assistant := Message{Role: "assistant", Content: stream.Content, ReasoningContent: stream.ReasoningContent, ToolCalls: stream.ToolCalls}
		if len(stream.ToolCalls) == 0 {
			esc.Stop()
			cancel()
			a.Session.LastReasoning = stream.ReasoningContent
			a.Session.Messages = append(a.Session.Messages, assistant)
			return nil
		}
		var toolMessages []Message
		for _, call := range stream.ToolCalls {
			if esc.WasCanceled() {
				esc.Stop()
				cancel()
				return fmt.Errorf("已按 Esc 取消本次请求")
			}
			if err := ctx.Err(); err != nil {
				cancel()
				return err
			}
			msg, canceled := executeToolWithAnimation(ctx, call, &perms)
			if canceled {
				err := ctx.Err()
				cancel()
				if err != nil {
					return err
				}
				return fmt.Errorf("已按 Esc 取消本次请求")
			}
			toolMessages = append(toolMessages, msg)
		}
		esc.Stop()
		cancel()
		a.Session.LastReasoning = stream.ReasoningContent
		a.Session.Messages = append(a.Session.Messages, assistant)
		a.Session.Messages = append(a.Session.Messages, toolMessages...)
	}
	return fmt.Errorf("工具调用循环超过限制：模型连续调用工具太多次，已中断以避免无限循环。当前结果已保存，可以让它基于现有结果继续总结")
}

func executeToolWithAnimation(ctx context.Context, call ToolCall, perms *PermissionState) (Message, bool) {
	printToolCallHeader(call)
	var anim *thinkingAnimation
	if shouldAnimateToolRun(call, perms) {
		anim = startToolRunAnimation(call.Function.Name)
	}
	result, canceled := runToolInteractive(ctx, call, perms)
	if anim != nil {
		anim.Stop()
	}
	msg := Message{Role: "tool", ToolCallID: call.ID, Content: limitToolOutput(result)}
	printToolCallOutput(call.Function.Name, result)
	return msg, canceled
}

func shouldAnimateToolRun(call ToolCall, perms *PermissionState) bool {
	if !dangerousTool(call.Function.Name) {
		return true
	}
	return perms != nil && (perms.DangerNext || perms.VeryDangerEnabled)
}

func needsInteractivePermission(call ToolCall, perms *PermissionState) bool {
	return dangerousTool(call.Function.Name) && (perms == nil || (!perms.DangerNext && !perms.VeryDangerEnabled))
}

// printToolCallHeader prints the Claude Code-style tree header:
// ● Read(main.go)
func printToolCallHeader(call ToolCall) {
	name := toolCallDisplayName(call.Function.Name)
	arg := toolCallArgSummary(call)
	if arg != "" {
		fmt.Printf("%s● %s(%s)%s\n", ansiBlue, name, arg, ansiReset)
	} else {
		fmt.Printf("%s● %s()%s\n", ansiBlue, name, ansiReset)
	}
}

func toolCallDisplayName(name string) string {
	switch name {
	case "get_cwd":
		return "CWD"
	case "read_file":
		return "Read"
	case "write_file":
		return "Write"
	case "delete_file":
		return "Delete"
	case "execute_bash":
		return "Bash"
	default:
		return name
	}
}

func toolCallArgSummary(call ToolCall) string {
	raw := call.Function.Arguments
	if raw == "" {
		return ""
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	switch call.Function.Name {
	case "read_file", "write_file", "delete_file":
		if p, ok := args["path"].(string); ok && p != "" {
			return filepath.Base(p)
		}
	case "execute_bash":
		if cmd, ok := args["command"].(string); ok {
			cmd = strings.TrimSpace(strings.ReplaceAll(cmd, "\n", " "))
			if len([]rune(cmd)) > 48 {
				cmd = string([]rune(cmd)[:48]) + "…"
			}
			return cmd
		}
	}
	return ""
}

// printToolCallOutput prints the ⎿ indented preview of tool output.
func printToolCallOutput(toolName, result string) {
	const maxLines = 5
	const firstPfx = "  ⎿  "
	const restPfx = "     "

	failed := strings.Contains(result, " denied:") ||
		strings.Contains(result, " failed:") ||
		strings.Contains(result, " canceled:") ||
		strings.Contains(result, "未授权") ||
		strings.Contains(result, "取消")

	color := ansiBlue
	if failed {
		color = ansiRed
	}

	// Strip "read_file ok path=...\n" prefix
	content := result
	if strings.HasPrefix(result, toolName+" ok") {
		if idx := strings.Index(result, "\n"); idx >= 0 {
			content = result[idx+1:]
		} else {
			// Single-line summary like "write_file ok path=... bytes=N"
			content = formatSingleLineSummary(toolName, result)
		}
	}

	content = strings.TrimRight(content, "\n")
	if content == "" || content == "ok" {
		return
	}

	lines := strings.Split(content, "\n")
	shown := lines
	remaining := 0
	if len(lines) > maxLines {
		shown = lines[:maxLines]
		remaining = len(lines) - maxLines
	}
	for i, line := range shown {
		if i == 0 {
			fmt.Printf("%s%s%s%s\n", color, firstPfx, line, ansiReset)
		} else {
			fmt.Printf("%s%s%s%s\n", ansiDim, restPfx, line, ansiReset)
		}
	}
	if remaining > 0 {
		fmt.Printf("%s%s… +%d lines%s\n", ansiDim, restPfx, remaining, ansiReset)
	}
}

func formatSingleLineSummary(toolName, result string) string {
	switch toolName {
	case "write_file":
		for _, part := range strings.Fields(result) {
			if strings.HasPrefix(part, "bytes=") {
				return "✓ " + strings.TrimPrefix(part, "bytes=") + " bytes written"
			}
		}
		return "✓ written"
	case "delete_file":
		return "✓ deleted"
	case "get_cwd":
		return result
	}
	return result
}

func (a *App) compressContext() error {
	if len(a.Session.Messages) == 0 {
		statusLine("当前没有可压缩的上下文。")
		return nil
	}
	prompt := []Message{
		systemPrompt(a.Config, a.Session.Model, PermissionState{}),
		{Role: "user", Content: "请把下面对话压缩成后续继续工作所需的中文摘要，保留目标、约束、关键文件、已完成工作和待办。\n\n" + messagesPlainText(a.Session.Messages)},
	}
	anim := startCompressAnimation()
	stream, err := quickChatWithCallbacks(a.Config, a.Session.Model, prompt, false, streamCallbacks{
		OnContent: func(text string) {
			anim.AddHiddenRunes(len([]rune(text)))
		},
	})
	anim.Stop()
	if err != nil {
		return err
	}
	a.Session.CompressedSummary = strings.TrimSpace(stream.Content)
	a.Session.Messages = nil
	statusLine("上下文已压缩。")
	return saveSession(a.Paths, &a.Session)
}

func messagesPlainText(messages []Message) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg.Content != "" {
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

type subTaskPlan struct {
	Tasks []string `json:"tasks"`
}

type subAgentRunResult struct {
	Agent BackgroundAgent
	Run   nativeResult
}

func (a *App) runSubAgentTurn(input string, perms PermissionState) error {
	statusLine("● Sub-agents")
	statusLine("  ├─ planning")
	planMessages := []Message{
		systemPrompt(a.Config, a.Session.Model, PermissionState{}),
		{Role: "user", Content: "把这个任务拆成 1 到 4 个可以并行处理的子任务，只输出 JSON，格式为 {\"tasks\":[\"...\"]}。任务：" + input},
	}
	stream, err := quickChat(a.Config, a.Session.Model, planMessages, false)
	if err != nil {
		return err
	}
	var plan subTaskPlan
	if err := json.Unmarshal([]byte(extractJSONObject(stream.Content)), &plan); err != nil || len(plan.Tasks) == 0 {
		plan.Tasks = []string{input}
	}
	if len(plan.Tasks) > 4 {
		plan.Tasks = plan.Tasks[:4]
	}
	exe, _ := os.Executable()
	a.Session.Messages = append(a.Session.Messages, Message{Role: "user", Content: input})
	nextID := a.Session.NextBackgroundAgentID
	if nextID <= 0 {
		nextID = nextBackgroundAgentID(a.Session.BackgroundAgents)
	}
	agents := make([]BackgroundAgent, 0, len(plan.Tasks))
	for i, task := range plan.Tasks {
		id := nextID
		nextID++
		agent := BackgroundAgent{
			ID:                id,
			Task:              task,
			Status:            "running",
			StartedAt:         time.Now(),
			DangerNext:        perms.DangerNext,
			VeryDangerEnabled: perms.VeryDangerEnabled,
		}
		agents = append(agents, agent)
		a.Session.BackgroundAgents = append(a.Session.BackgroundAgents, agent)
		branch := "├"
		if i == len(plan.Tasks)-1 {
			branch = "╰"
		}
		label := "normal"
		if perms.DangerNext || perms.VeryDangerEnabled {
			label = "danger"
		}
		statusLine(fmt.Sprintf("  %s─ Agent #%d running [%s] %s", branch, id, label, oneLinePreview(task, 64)))
	}
	a.Session.NextBackgroundAgentID = nextID
	_ = saveSession(a.Paths, &a.Session)

	ctx, cancel := context.WithCancel(context.Background())
	esc := startEscWatcher(cancel)
	anim := startSubAgentAnimation(len(agents))
	results := make(chan subAgentRunResult, len(agents))
	for _, agent := range agents {
		go func(agent BackgroundAgent) {
			results <- subAgentRunResult{
				Agent: agent,
				Run:   runSubAgentProcess(ctx, exe, agent.Task, a.Paths.ConfigFile, sessionPath(a.Paths, a.Session.ID), perms),
			}
		}(agent)
	}
	for range agents {
		result := <-results
		status, output, errText := subAgentResultStatus(ctx, result.Run)
		a.updateBackgroundAgent(result.Agent.ID, status, output, errText)
		anim.Mark(status)
	}
	wasCanceled := esc.WasCanceled() || ctx.Err() != nil
	esc.Stop()
	cancel()
	anim.Stop()
	a.printBackgroundAgentSummary(agents)
	if wasCanceled {
		a.Session.Messages = append(a.Session.Messages, Message{Role: "assistant", Content: "已按 Esc 取消所有子代理，未生成最终答复。"})
		return fmt.Errorf("已按 Esc 取消所有子代理")
	}
	if merged := a.consumeCompletedSubAgentResults(); merged != "" {
		a.Session.Messages = append(a.Session.Messages, Message{Role: "user", Content: merged})
	}
	statusLine("● Sub-agents complete; 主模型正在整合结果")
	return a.completeWithTools(perms)
}

func (a *App) startBackgroundSubAgent(exe, task string, perms PermissionState) int {
	a.mu.Lock()
	id := a.Session.NextBackgroundAgentID
	if id <= 0 {
		id = nextBackgroundAgentID(a.Session.BackgroundAgents)
	}
	a.Session.NextBackgroundAgentID = id + 1
	a.Session.BackgroundAgents = append(a.Session.BackgroundAgents, BackgroundAgent{
		ID:                id,
		Task:              task,
		Status:            "running",
		StartedAt:         time.Now(),
		DangerNext:        perms.DangerNext,
		VeryDangerEnabled: perms.VeryDangerEnabled,
	})
	a.mu.Unlock()
	go a.runBackgroundSubAgent(id, exe, task, perms)
	return id
}

func nextBackgroundAgentID(agents []BackgroundAgent) int {
	maxID := 0
	for _, agent := range agents {
		if agent.ID > maxID {
			maxID = agent.ID
		}
	}
	return maxID + 1
}

func (a *App) runBackgroundSubAgent(id int, exe, task string, perms PermissionState) {
	r := runSubAgentProcess(context.Background(), exe, task, a.Paths.ConfigFile, sessionPath(a.Paths, a.Session.ID), perms)
	a.mu.Lock()
	defer a.mu.Unlock()
	status, result, errText := subAgentResultStatus(context.Background(), r)
	for i := range a.Session.BackgroundAgents {
		if a.Session.BackgroundAgents[i].ID == id {
			a.Session.BackgroundAgents[i].Status = status
			a.Session.BackgroundAgents[i].Result = result
			a.Session.BackgroundAgents[i].Error = errText
			a.Session.BackgroundAgents[i].FinishedAt = time.Now()
			break
		}
	}
	_ = saveSession(a.Paths, &a.Session)
}

func runSubAgentProcess(ctx context.Context, exe, task, configPath, sessionPath string, perms PermissionState) nativeResult {
	return nativeRunSubAgentCancelable(ctx, exe, task, configPath, sessionPath, perms)
}

func subAgentResultStatus(ctx context.Context, r nativeResult) (string, string, string) {
	result := strings.TrimSpace(r.Output)
	if ctx.Err() != nil || strings.TrimSpace(r.Error) == "canceled" {
		return "canceled", result, "canceled"
	}
	if r.Code != 0 {
		errText := strings.TrimSpace(r.Error)
		if result != "" {
			errText = strings.TrimSpace(errText + "\n" + result)
		}
		return "failed", result, errText
	}
	return "done", result, ""
}

func (a *App) updateBackgroundAgent(id int, status, result, errText string) {
	for i := range a.Session.BackgroundAgents {
		if a.Session.BackgroundAgents[i].ID == id {
			a.Session.BackgroundAgents[i].Status = status
			a.Session.BackgroundAgents[i].Result = result
			a.Session.BackgroundAgents[i].Error = errText
			a.Session.BackgroundAgents[i].FinishedAt = time.Now()
			a.Session.BackgroundAgents[i].Notified = true
			return
		}
	}
}

func (a *App) printBackgroundAgentSummary(started []BackgroundAgent) {
	byID := map[int]BackgroundAgent{}
	for _, agent := range a.Session.BackgroundAgents {
		byID[agent.ID] = agent
	}
	for i, startedAgent := range started {
		agent := byID[startedAgent.ID]
		branch := "├"
		if i == len(started)-1 {
			branch = "╰"
		}
		switch agent.Status {
		case "done":
			statusLine(fmt.Sprintf("  %s─ Agent #%d done · %s", branch, agent.ID, oneLinePreview(agent.Result, 72)))
		case "failed":
			dangerLine(fmt.Sprintf("  %s─ Agent #%d failed · %s", branch, agent.ID, oneLinePreview(agent.Error, 72)))
		case "canceled":
			dangerLine(fmt.Sprintf("  %s─ Agent #%d canceled", branch, agent.ID))
		}
	}
}

func (a *App) printBackgroundAgentNotifications() {
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	for i := range a.Session.BackgroundAgents {
		agent := &a.Session.BackgroundAgents[i]
		if agent.Notified || (agent.Status != "done" && agent.Status != "failed" && agent.Status != "canceled") {
			continue
		}
		if agent.Status == "done" {
			statusLine(fmt.Sprintf("● Agent #%d done · %s", agent.ID, oneLinePreview(agent.Result, 72)))
		} else if agent.Status == "failed" {
			dangerLine(fmt.Sprintf("● Agent #%d failed · %s", agent.ID, oneLinePreview(agent.Error, 72)))
		} else {
			dangerLine(fmt.Sprintf("● Agent #%d canceled", agent.ID))
		}
		agent.Notified = true
		changed = true
	}
	if changed {
		_ = saveSession(a.Paths, &a.Session)
	}
}

func (a *App) consumeCompletedSubAgentResults() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var blocks []string
	for i := range a.Session.BackgroundAgents {
		agent := &a.Session.BackgroundAgents[i]
		if agent.Merged || (agent.Status != "done" && agent.Status != "failed" && agent.Status != "canceled") {
			continue
		}
		if agent.Status == "done" {
			blocks = append(blocks, fmt.Sprintf("Agent #%d result (%s):\n%s", agent.ID, agent.Task, agent.Result))
		} else if agent.Status == "failed" {
			blocks = append(blocks, fmt.Sprintf("Agent #%d failed (%s):\n%s", agent.ID, agent.Task, agent.Error))
		} else {
			blocks = append(blocks, fmt.Sprintf("Agent #%d canceled (%s)", agent.ID, agent.Task))
		}
		agent.Merged = true
	}
	if len(blocks) == 0 {
		return ""
	}
	return "后台子代理返回结果：\n" + strings.Join(blocks, "\n\n") + "\n\n请结合这些结果回答下一条用户消息。"
}

func (a *App) showFirstTurnTips() {
	if a.Session.TipsShown {
		return
	}
	a.Session.TipsShown = true
	statusLine("Tips: /rename 可重命名当前会话，/恢复 可找回历史对话，/子代理 可把下一次任务交给后台 Agent。")
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return text
}

func runSubAgentMode(args []string) int {
	task := ""
	configPath := ""
	sessionPathValue := ""
	perms := PermissionState{}
	if len(args) > 0 {
		task = args[0]
	}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case "--session":
			if i+1 < len(args) {
				sessionPathValue = args[i+1]
				i++
			}
		case "--danger-next":
			perms.DangerNext = true
		case "--very-danger":
			perms.VeryDangerEnabled = true
		}
	}
	if configPath == "" {
		fmt.Println("missing config")
		return 2
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Println(err)
		return 2
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Println(err)
		return 2
	}
	var base []Message
	if sessionPathValue != "" {
		if data, err := os.ReadFile(sessionPathValue); err == nil {
			var s Session
			if json.Unmarshal(data, &s) == nil {
				base = s.Messages
			}
		}
	}
	result, err := runSubAgentTask(cfg, base, task, perms)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(result)
	return 0
}

func runSubAgentTask(cfg Config, base []Message, task string, perms PermissionState) (string, error) {
	s := newSession(cfg)
	s.Messages = append(s.Messages, base...)
	s.Messages = append(s.Messages, Message{Role: "user", Content: "你是一个子代理。只处理这个独立子任务，输出简洁中文结果：\n" + task})
	for step := 0; step < 12; step++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		stream, err := chatStream(ctx, cfg, cfg.DefaultModel, buildMessages(cfg, s, perms), toolSpecs(), false, streamCallbacks{})
		if err != nil {
			cancel()
			return "", err
		}
		assistant := Message{Role: "assistant", Content: stream.Content, ReasoningContent: stream.ReasoningContent, ToolCalls: stream.ToolCalls}
		s.Messages = append(s.Messages, assistant)
		if len(stream.ToolCalls) == 0 {
			cancel()
			return strings.TrimSpace(stream.Content), nil
		}
		for _, call := range stream.ToolCalls {
			result := runToolContext(ctx, call, perms)
			if err := ctx.Err(); err != nil {
				cancel()
				return "", err
			}
			s.Messages = append(s.Messages, Message{Role: "tool", ToolCallID: call.ID, Content: limitToolOutput(result)})
		}
		cancel()
	}
	return "", fmt.Errorf("子代理工具调用循环超过限制")
}
