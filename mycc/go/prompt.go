package main

import (
	"fmt"
	"os"
	"strings"
)

func systemPrompt(cfg Config, model string, perms PermissionState) Message {
	if model == "" {
		model = cfg.DefaultModel
	}
	style := map[string]string{
		"简洁优先": "回复尽量短，能短则短，不废话。",
		"适中":   "该详细的地方详细，该短的地方短。",
		"详细":   "宁可多说，不要漏掉重要信息。",
	}[cfg.ReplyStyle]
	if style == "" {
		style = "该详细的地方详细，该短的地方短。"
	}
	displayHint := "显示模式：炫彩。CLI 会为普通文本做终端颜色渲染；你不要输出原始 ANSI 转义码。"
	if cfg.DisplayMode == displayPlain {
		displayHint = "显示模式：简洁。减少装饰，仍保持清楚自然；不要输出原始 ANSI 转义码。"
	}
	cwd, _ := os.Getwd()
	permText := "当前是普通权限：可以读取文件、查看当前工作目录，不能写文件、删除文件或执行 shell。"
	if perms.DangerNext {
		permText = "当前下一次任务有危险权限：允许写文件、删除文件和执行 shell；任务结束后会撤销。"
	}
	if perms.VeryDangerEnabled {
		permText = "当前终端已开启持续危险权限：允许写文件、删除文件和执行 shell。"
	}
	content := fmt.Sprintf(`你是 Deepcode，一个嵌入 Linux 终端 CLI 的 AI 编程助手。
必须用中文回复用户。技术术语、代码、路径、命令名、API 名称可以保留英文，但解释和普通句子必须是中文。
如果 API 产生 reasoning_content/thinking 内容，思考内容也必须写中文。
语气：自然、直接、有用，普通回复保持干净克制。
不要使用 emoji，除非用户明确要求或用户先使用。
不要在每次打招呼时介绍自己；只有用户问你是谁，或确实有帮助时才介绍。
终端支持 ANSI 颜色；你不要输出原始 ANSI 转义码。
普通 prose 不要使用 Markdown 格式：不要标题、不要加粗标记、不要行内反引号、不要列表，除非用户要求结构化列表。代码块仍然使用带语言名的 fenced Markdown。
需要使用工具时，先用一句简短中文说明你要做什么，再调用工具。
可用工具：get_cwd、read_file、write_file、delete_file、execute_bash。
文件工具只允许访问当前工作目录内的路径；使用当前目录内路径，不要请求 ../ 路径或无关绝对路径。
需要知道当前工作目录时使用 get_cwd。
如果写入、删除或 shell 工具需要授权，照常调用工具；CLI 会询问用户。不要让用户输入授权命令，也不要提隐藏内部命令。
工具被拒绝或取消时，用中文说明情况，并继续尝试不需要危险权限的替代方案；不要切换成英文道歉。
子代理可在任务独立或用户要求 /子代理 时使用。
如果用户询问当前 provider、model 或配置，按当前配置如实回答；不能确认就说不能确认，不要编造或隐瞒。
回复风格：%s
%s
Provider：%s
Model：%s
当前工作目录：%s
权限：%s`, style, displayHint, cfg.Provider, model, cwd, permText)
	return Message{Role: "system", Content: content}
}

func buildMessages(cfg Config, s Session, perms PermissionState) []Message {
	out := []Message{systemPrompt(cfg, s.Model, perms)}
	if s.CompressedSummary != "" {
		out = append(out, Message{Role: "system", Content: "此前上下文摘要：" + s.CompressedSummary})
	}
	if s.RestoredNotice != "" {
		out = append(out, Message{Role: "system", Content: s.RestoredNotice})
	}
	if status := backgroundAgentStatusContext(s); status != "" {
		out = append(out, Message{Role: "system", Content: status})
	}
	out = append(out, s.Messages...)
	return out
}

func backgroundAgentStatusContext(s Session) string {
	if len(s.BackgroundAgents) == 0 {
		return ""
	}
	start := len(s.BackgroundAgents) - 8
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	b.WriteString("子代理真实状态如下；如果用户询问子代理是否在运行、完成或失败，以此为准：")
	for _, agent := range s.BackgroundAgents[start:] {
		b.WriteString("\n- Agent #")
		b.WriteString(fmt.Sprintf("%d", agent.ID))
		b.WriteString(" ")
		b.WriteString(agent.Status)
		b.WriteString(": ")
		b.WriteString(oneLinePreview(agent.Task, 60))
	}
	return b.String()
}
