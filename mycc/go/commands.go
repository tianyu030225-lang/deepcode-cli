package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type commandInfo struct {
	Name        string
	Description string
	Hidden      bool
	Canonical   string
}

var slashCommandInfos = []commandInfo{
	{Name: "/compact", Description: "压缩此线程的上下文", Canonical: "/compact"},
	{Name: "/压缩", Description: "压缩此线程的上下文", Canonical: "/compact"},
	{Name: "/agent", Description: "强制模型启动子代理完成下一次任务", Canonical: "/agent"},
	{Name: "/子代理", Description: "强制模型启动子代理完成下一次任务", Canonical: "/agent"},
	{Name: "/thinking", Description: "开关 DeepSeek thinking 模式", Canonical: "/thinking"},
	{Name: "/思考", Description: "开关 DeepSeek thinking 模式", Canonical: "/thinking"},
	{Name: "/model", Description: "切换默认/轻量/最强模型", Canonical: "/model"},
	{Name: "/模型", Description: "切换默认/轻量/最强模型", Canonical: "/model"},
	{Name: "/show", Description: "切换炫彩/简洁显示模式", Canonical: "/show"},
	{Name: "/显示", Description: "切换炫彩/简洁显示模式", Canonical: "/show"},
	{Name: "/show-thinking", Description: "展示最近一次保存的思考内容", Canonical: "/show-thinking"},
	{Name: "/review", Description: "审查当前工作目录的所有代码", Canonical: "/review"},
	{Name: "/审查", Description: "审查当前工作目录的所有代码", Canonical: "/review"},
	{Name: "/resume", Description: "查找并恢复本地对话", Canonical: "/resume"},
	{Name: "/恢复", Description: "查找并恢复本地对话", Canonical: "/resume"},
	{Name: "/rename", Description: "重命名当前会话", Canonical: "/rename"},
	{Name: "/重命名", Description: "重命名当前会话", Canonical: "/rename"},
	{Name: "/exit", Description: "退出 CLI", Canonical: "/exit"},
	{Name: "/退出", Description: "退出 CLI", Canonical: "/exit"},
	{Name: "/danger", Description: "下一次任务允许写文件、删除文件、执行 shell", Canonical: "/danger"},
	{Name: "/危险", Description: "下一次任务允许写文件、删除文件、执行 shell", Canonical: "/danger"},
	{Name: "/very-danger", Description: "当前终端持续允许危险操作", Hidden: true, Canonical: "/very-danger"},
}

func visibleCommandMatches(prefix string) []commandInfo {
	var out []commandInfo
	for _, cmd := range slashCommandInfos {
		if cmd.Hidden {
			continue
		}
		if prefix == "" || prefix == "/" || len(prefix) <= len(cmd.Name) && cmd.Name[:len(prefix)] == prefix {
			out = append(out, cmd)
		}
	}
	return out
}

func (a *App) handleCommand(cmd string) (bool, error) {
	name, arg := splitSlashCommand(cmd)
	switch canonicalCommand(name) {
	case "/compact":
		return false, a.compressContext()
	case "/agent":
		a.Session.ForceSubAgentNext = true
		statusLine("下一次任务会强制启动子代理。")
		return false, saveSession(a.Paths, &a.Session)
	case "/thinking":
		a.Session.ThinkingEnabled = !a.Session.ThinkingEnabled
		if a.Config.Provider != providerDeepSeek {
			dangerLine("您使用的非 DeepSeek 模型，可能无法启动思考模式。")
		}
		if a.Session.ThinkingEnabled {
			statusLine("思考模式已开启，reasoning_content 会保存到会话里。")
		} else {
			statusLine("思考模式已关闭。")
		}
		return false, saveSession(a.Paths, &a.Session)
	case "/model":
		return false, a.switchModel()
	case "/show":
		return false, a.switchDisplayMode()
	case "/show-thinking":
		if a.Session.LastReasoning == "" {
			statusLine("当前还没有保存的思考内容。")
		} else {
			fmt.Println(ansiBlue + "最近一次思考内容：" + ansiReset)
			fmt.Println(a.reasoningForDisplay())
		}
		return false, nil
	case "/review":
		return false, a.runReviewCommand()
	case "/resume":
		return false, a.resumeSession()
	case "/rename":
		return false, a.renameSession(arg)
	case "/exit":
		return true, saveSession(a.Paths, &a.Session)
	case "/danger":
		a.Session.DangerNext = true
		dangerLine("危险权限已开启：仅下一次用户任务可写文件、删除文件、执行 shell。")
		return false, saveSession(a.Paths, &a.Session)
	case "/very-danger":
		a.Session.VeryDangerEnabled = true
		dangerLine("持续危险权限已开启：当前终端会话持续允许写/删/执行。")
		return false, saveSession(a.Paths, &a.Session)
	default:
		statusLine("未知命令：" + cmd)
		return false, nil
	}
}

func splitSlashCommand(cmd string) (string, string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd, ""
	}
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
	return parts[0], arg
}

func (a *App) runReviewCommand() error {
	prompt, err := codeReviewCommandPrompt()
	if err != nil {
		return err
	}
	return a.runUserTurn(prompt)
}

func codeReviewCommandPrompt() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	files, skipped, err := collectReviewFiles(cwd)
	if err != nil {
		return "", err
	}
	const maxTotalRunes = 180000
	const maxFileBytes = 80000
	remaining := maxTotalRunes
	var b strings.Builder
	b.WriteString("请审查当前工作目录的所有代码。\n\n")
	b.WriteString("审查目标：找出真实的 bug、行为回归、安全风险、并发/资源泄漏、错误处理缺陷、跨平台问题，以及明显缺失的测试。不要做泛泛的代码风格点评。\n")
	b.WriteString("工作要求：\n")
	b.WriteString("1. 先基于下面的文件清单和代码快照审查整个项目。\n")
	b.WriteString("2. 如果某个文件被截断或没有包含完整内容，请优先用 read_file 读取该文件后再判断。\n")
	b.WriteString("3. 输出中文，按严重程度排序。每条问题必须包含文件路径、具体位置或函数名、影响、建议修复方式。\n")
	b.WriteString("4. 没有确认的问题不要强行编造；如果没有发现高可信问题，就明确说未发现阻塞问题，并列出剩余测试风险。\n")
	b.WriteString("5. 本命令只做审查，不要修改文件，除非用户后续明确要求。\n\n")
	b.WriteString("当前工作目录：")
	b.WriteString(cwd)
	b.WriteString("\n\n文件清单：\n")
	for _, file := range files {
		b.WriteString("- ")
		b.WriteString(file)
		b.WriteByte('\n')
	}
	if len(skipped) > 0 {
		b.WriteString("\n未纳入快照的目录/文件：\n")
		for _, item := range skipped {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n代码快照：\n")
	for _, file := range files {
		if remaining <= 0 {
			b.WriteString("\n[快照总长度达到上限；如需继续审查，请用 read_file 读取文件清单中的剩余文件。]\n")
			break
		}
		path := filepath.Join(cwd, file)
		data, err := readReviewFile(path, maxFileBytes)
		if err != nil {
			b.WriteString("\n--- ")
			b.WriteString(file)
			b.WriteString(" ---\n[读取失败：")
			b.WriteString(err.Error())
			b.WriteString("]\n")
			continue
		}
		truncated := len(data) > maxFileBytes
		if truncated {
			data = data[:maxFileBytes]
			// Drop only an incomplete trailing rune at the snapshot boundary.
			start := len(data) - 1
			for start >= 0 && !utf8.RuneStart(data[start]) {
				start--
			}
			if start >= 0 && !utf8.FullRune(data[start:]) {
				data = data[:start]
			}
		}
		if !utf8.Valid(data) {
			continue
		}
		text := string(data)
		runes := []rune(text)
		if len(runes) > remaining {
			runes = runes[:remaining]
			text = string(runes)
			truncated = true
		}
		b.WriteString("\n--- ")
		b.WriteString(file)
		b.WriteString(" ---\n")
		b.WriteString(text)
		if truncated {
			b.WriteString("\n[此文件快照被截断；如需完整判断，请用 read_file 读取：")
			b.WriteString(path)
			b.WriteString("]\n")
		}
		remaining -= len(runes)
	}
	return b.String(), nil
}

func readReviewFile(path string, maxBytes int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("review only supports regular files")
	}
	file, err := nativeOpenWorkspaceReadFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// The extra byte lets the existing snapshot code label truncated files.
	return io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
}

func collectReviewFiles(root string) ([]string, []string, error) {
	var files []string
	var skipped []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			skipped = append(skipped, path+": "+err.Error())
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipReviewDir(name) {
				if path != root {
					rel, _ := filepath.Rel(root, path)
					skipped = append(skipped, rel+"/")
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !entry.Type().IsRegular() || !isReviewCodeFile(name) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	sort.Strings(skipped)
	return files, skipped, err
}

func shouldSkipReviewDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "third_party", "build", "dist", "target", ".next", ".cache":
		return true
	default:
		return false
	}
}

func isReviewCodeFile(name string) bool {
	switch name {
	case "Makefile", "Dockerfile", "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "requirements.txt", "Cargo.toml", "Cargo.lock":
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".c", ".h", ".cc", ".cpp", ".hpp", ".rs", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".java", ".kt", ".swift", ".sh", ".bash", ".zsh", ".sql", ".html", ".css", ".scss", ".json", ".yaml", ".yml", ".toml", ".md":
		return true
	default:
		return false
	}
}

func canonicalCommand(cmd string) string {
	for _, item := range slashCommandInfos {
		if item.Name == cmd {
			if item.Canonical != "" {
				return item.Canonical
			}
			return item.Name
		}
	}
	return cmd
}

func (a *App) switchDisplayMode() error {
	modes := []string{displayColorful, displayPlain}
	labels := []string{"炫彩模式: 多色终端渲染", "简洁模式: 干净文本输出"}
	idx, err := chooseMenu("选择显示模式", labels)
	if err != nil {
		return err
	}
	a.Config.DisplayMode = modes[idx]
	if err := saveConfig(a.Paths, a.Config); err != nil {
		return err
	}
	clearScreen()
	renderWelcomePanel(a.Config, a.Session)
	statusLine("显示模式已切换为：" + a.Config.DisplayMode)
	return nil
}

func (a *App) switchModel() error {
	labels := []string{
		"默认模型: " + a.Config.DefaultModel,
		"轻量模型: " + a.Config.LightModel,
		"最强模型: " + a.Config.StrongModel,
	}
	models := []string{a.Config.DefaultModel, a.Config.LightModel, a.Config.StrongModel}
	idx, err := chooseMenu("选择当前会话模型", labels)
	if err != nil {
		return err
	}
	a.Session.Model = models[idx]
	if err := saveSession(a.Paths, &a.Session); err != nil {
		return err
	}
	clearScreen()
	renderWelcomePanel(a.Config, a.Session)
	statusLine("当前会话模型已切换为：" + a.Session.Model)
	return nil
}

func (a *App) renameSession(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		var err error
		name, err = promptLine("新的会话名称: ", false)
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
	}
	if name == "" {
		return fmt.Errorf("会话名称不能为空")
	}
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60])
	}
	a.Session.Title = name
	if err := saveSession(a.Paths, &a.Session); err != nil {
		return err
	}
	clearScreen()
	renderWelcomePanel(a.Config, a.Session)
	statusLine("当前会话已重命名为：" + name)
	return nil
}

func (a *App) reasoningForDisplay() string {
	if containsCJK(a.Session.LastReasoning) {
		return a.Session.LastReasoning
	}
	statusLine("思考内容是英文，正在整理成中文...")
	msgs := []Message{
		{Role: "system", Content: "你只负责把英文 reasoning_content 翻译并整理成自然、简洁的中文。不要添加新信息。"},
		{Role: "user", Content: a.Session.LastReasoning},
	}
	stream, err := quickChat(a.Config, a.Session.Model, msgs, false)
	if err != nil || stream.Content == "" {
		return a.Session.LastReasoning
	}
	return stream.Content
}

func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func (a *App) resumeSession() error {
	sessions, err := listSessions(a.Paths)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		statusLine("还没有历史对话。")
		return nil
	}
	opts := make([]string, len(sessions))
	for i, s := range sessions {
		opts[i] = formatSessionOption(s)
	}
	idx, err := chooseMenu("选择要恢复的对话", opts)
	if err != nil {
		return err
	}
	loaded, err := loadSession(a.Paths, sessions[idx].ID)
	if err != nil {
		return err
	}
	loaded.DangerNext = false
	loaded.VeryDangerEnabled = a.Session.VeryDangerEnabled
	loaded.RestoredNotice = restoredSessionNotice(loaded)
	a.Session = loaded
	clearScreen()
	renderWelcomePanel(a.Config, a.Session)
	printRestoredSessionSummary(loaded)
	return nil
}

func formatSessionOption(s SessionInfo) string {
	label := fmt.Sprintf("%s · %s · %d条 · %s", s.UpdatedAt.Format("01-02 15:04"), s.Model, s.MessageCount, s.Title)
	if s.LastUserText != "" {
		label += " · 最近: " + s.LastUserText
	}
	return label
}

func restoredSessionNotice(s Session) string {
	notice := fmt.Sprintf(
		"当前 CLI 刚刚恢复了一个本地历史对话。请基于后续 messages 中的历史继续回答，不要把它当作新对话。恢复标题：%s。恢复时间：%s。Provider：%s。Model：%s。历史消息数：%d。",
		fallbackText(s.Title, "未命名对话"),
		s.UpdatedAt.Format("2006-01-02 15:04"),
		fallbackText(s.Provider, "unknown"),
		fallbackText(s.Model, "unknown"),
		len(s.Messages),
	)
	if s.CompressedSummary != "" {
		notice += "该会话包含压缩摘要，摘要会作为系统上下文提供。"
	}
	return notice
}

func printRestoredSessionSummary(s Session) {
	fmt.Println(ansiBlue + "已恢复历史对话" + ansiReset)
	fmt.Println("  标题: " + fallbackText(s.Title, "未命名对话"))
	fmt.Println("  时间: " + s.UpdatedAt.Format("2006-01-02 15:04"))
	fmt.Println("  模型: " + fallbackText(s.Provider, "unknown") + " / " + fallbackText(s.Model, "unknown"))
	fmt.Printf("  消息: %d 条\n", len(s.Messages))
	if s.CompressedSummary != "" {
		fmt.Println("  摘要: " + oneLinePreview(s.CompressedSummary, 72))
	}
	if last := lastUserText(s.Messages); last != "" {
		fmt.Println("  最近用户: " + last)
	}
	if last := lastAssistantText(s.Messages); last != "" {
		fmt.Println("  最近助手: " + last)
	}
}
