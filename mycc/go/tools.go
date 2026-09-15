package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type PermissionState struct {
	DangerNext        bool
	VeryDangerEnabled bool
}

func toolSpecs() []ToolSpec {
	stringParam := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	return []ToolSpec{
		{
			Type: "function",
			Function: ToolSpecFunction{
				Name:        "get_cwd",
				Description: "获取 Deepcode CLI 进程当前工作目录。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolSpecFunction{
				Name:        "read_file",
				Description: "读取当前工作目录内的 UTF-8 文本文件。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"path": stringParam("要读取的文件路径，相对于当前工作目录。")},
					"required":   []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolSpecFunction{
				Name:        "write_file",
				Description: "覆盖写入当前工作目录内的 UTF-8 文本文件，需要危险权限。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"path": stringParam("要写入的文件路径，相对于当前工作目录。"), "content": stringParam("完整文件内容。")},
					"required":   []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolSpecFunction{
				Name:        "delete_file",
				Description: "删除当前工作目录内的文件，需要危险权限。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"path": stringParam("要删除的文件路径，相对于当前工作目录。")},
					"required":   []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolSpecFunction{
				Name:        "execute_bash",
				Description: "执行 bash 命令并捕获 stdout/stderr，需要危险权限。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"command": stringParam("要执行的 bash 命令。")},
					"required":   []string{"command"},
				},
			},
		},
	}
}

func runToolInteractive(ctx context.Context, call ToolCall, perms *PermissionState) (string, bool) {
	args, errText := parseToolArgs(call)
	if errText != "" {
		return errText, false
	}
	if perms == nil {
		perms = &PermissionState{}
	}
	callPerms := *perms
	if dangerousTool(call.Function.Name) && !perms.DangerNext && !perms.VeryDangerEnabled {
		switch promptDangerPermission(call, args) {
		case dangerPermissionAllowed:
		case dangerPermissionCanceled:
			return dangerActionText(call.Function.Name) + "已取消：用户按 Esc 取消本次任务", true
		default:
			return dangerActionText(call.Function.Name) + "已拒绝：用户未授权本次危险操作", false
		}
		callPerms.DangerNext = true
	}
	toolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	esc := startEscWatcher(cancel)
	result := runToolWithArgs(toolCtx, call, args, callPerms)
	esc.Stop()
	return result, esc.WasCanceled() || toolCtx.Err() != nil
}

func limitToolOutput(text string) string {
	const maxRunes = 120000
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "\n\n[tool output truncated by Deepcode: 输出太长，已截断，必要时请读取更小的范围或执行更精确的命令]"
}

func parseToolArgs(call ToolCall) (map[string]string, string) {
	rawArgs := call.Function.Arguments
	if rawArgs == "" {
		rawArgs = "{}"
	}
	parsedArgs := rawArgs
	if call.Function.Name == "get_cwd" ||
		call.Function.Name == "read_file" ||
		call.Function.Name == "write_file" ||
		call.Function.Name == "delete_file" ||
		call.Function.Name == "execute_bash" {
		r := nativeParseToolArgsJSON(call.Function.Name, rawArgs)
		if err := r.err(); err != nil {
			return nil, err.Error()
		}
		parsedArgs = r.Output
	}
	switch call.Function.Name {
	case "get_cwd":
		var args struct{}
		if errText := unmarshalToolArgs(parsedArgs, &args); errText != "" {
			return nil, errText
		}
		return map[string]string{}, ""
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if errText := unmarshalToolArgs(parsedArgs, &args); errText != "" {
			return nil, errText
		}
		path, errText := workspaceFilePath(args.Path)
		if errText != "" {
			return nil, errText
		}
		return map[string]string{"path": path}, ""
	case "write_file":
		var args struct {
			Path    string  `json:"path"`
			Content *string `json:"content"`
		}
		if errText := unmarshalToolArgs(parsedArgs, &args); errText != "" {
			return nil, errText
		}
		path, errText := workspaceFilePath(args.Path)
		if errText != "" {
			return nil, errText
		}
		if args.Content == nil {
			return nil, "工具参数 content 不能为空"
		}
		return map[string]string{"path": path, "content": *args.Content}, ""
	case "delete_file":
		var args struct {
			Path string `json:"path"`
		}
		if errText := unmarshalToolArgs(parsedArgs, &args); errText != "" {
			return nil, errText
		}
		path, errText := workspaceFilePath(args.Path)
		if errText != "" {
			return nil, errText
		}
		return map[string]string{"path": path}, ""
	case "execute_bash":
		var args struct {
			Command string `json:"command"`
		}
		if errText := unmarshalToolArgs(parsedArgs, &args); errText != "" {
			return nil, errText
		}
		if strings.TrimSpace(args.Command) == "" {
			return nil, "工具参数 command 不能为空"
		}
		return map[string]string{"command": args.Command}, ""
	default:
		args := map[string]string{}
		if errText := unmarshalToolArgs(rawArgs, &args); errText != "" {
			return nil, errText
		}
		return args, ""
	}
}

func unmarshalToolArgs(rawArgs string, dst interface{}) string {
	if err := json.Unmarshal([]byte(rawArgs), dst); err != nil {
		return "工具参数 JSON 解析失败: " + err.Error()
	}
	return ""
}

func workspaceFilePath(path string) (string, string) {
	r := nativeWorkspaceFilePath(path)
	if err := r.err(); err != nil {
		return "", err.Error()
	}
	return r.Output, ""
}

func runTool(call ToolCall, perms PermissionState) string {
	return runToolContext(context.Background(), call, perms)
}

func runToolContext(ctx context.Context, call ToolCall, perms PermissionState) string {
	args, errText := parseToolArgs(call)
	if errText != "" {
		return errText
	}
	return runToolWithArgs(ctx, call, args, perms)
}

func runToolWithArgs(ctx context.Context, call ToolCall, args map[string]string, perms PermissionState) string {
	if err := ctx.Err(); err != nil {
		return "tool canceled: " + err.Error()
	}
	allowedDanger := perms.DangerNext || perms.VeryDangerEnabled
	switch call.Function.Name {
	case "get_cwd":
		cwd, err := os.Getwd()
		if err != nil {
			return "get_cwd failed: " + err.Error()
		}
		return cwd
	case "read_file":
		r := nativeReadWorkspaceFile(args["path"])
		if err := r.err(); err != nil {
			return "read_file failed: " + err.Error()
		}
		return fmt.Sprintf("read_file ok path=%s\n%s", args["path"], r.Output)
	case "write_file":
		if !allowedDanger {
			return "write_file denied: 需要用户授权危险权限"
		}
		r := nativeWriteWorkspaceFile(args["path"], args["content"])
		if err := r.err(); err != nil {
			return "write_file failed: " + err.Error()
		}
		return fmt.Sprintf("write_file ok path=%s bytes=%d", args["path"], len([]byte(args["content"])))
	case "delete_file":
		if !allowedDanger {
			return "delete_file denied: 需要用户授权危险权限"
		}
		r := nativeDeleteWorkspaceFile(args["path"])
		if err := r.err(); err != nil {
			return "delete_file failed: " + err.Error()
		}
		return fmt.Sprintf("delete_file ok path=%s", args["path"])
	case "execute_bash":
		if !allowedDanger {
			return "execute_bash denied: 需要用户授权危险权限"
		}
		r := nativeExecuteBashCancelable(ctx, args["command"])
		if r.Code != 0 {
			return fmt.Sprintf("execute_bash exit=%d error=%s\n%s", r.ExitCode, r.Error, r.Output)
		}
		return r.Output
	default:
		return "unknown tool: " + call.Function.Name
	}
}
