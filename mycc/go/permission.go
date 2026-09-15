package main

import (
	"os"
	"strings"
)

type dangerPermissionResult int

const (
	dangerPermissionDenied dangerPermissionResult = iota
	dangerPermissionAllowed
	dangerPermissionCanceled
)

func dangerousTool(name string) bool {
	return name == "write_file" || name == "delete_file" || name == "execute_bash"
}

func promptDangerPermission(call ToolCall, args map[string]string) dangerPermissionResult {
	action := dangerActionText(call.Function.Name)
	target := dangerTargetText(call.Function.Name, args)
	label := action
	if target != "" {
		if len([]rune(target)) > 48 {
			target = string([]rune(target)[:48]) + "…"
		}
		label = action + "(" + target + ")"
	}
	r := nativePromptDangerPermission(label)
	if err := r.err(); err != nil {
		return dangerPermissionCanceled
	}
	switch strings.TrimSpace(r.Output) {
	case "allowed":
		return dangerPermissionAllowed
	case "denied":
		return dangerPermissionDenied
	default:
		return dangerPermissionCanceled
	}
}

func drainPendingInput() {
	_ = withRawModeTimed(func() error {
		buf := make([]byte, 1)
		for {
			n, err := readStdinByte(buf)
			if err != nil || n == 0 {
				return nil
			}
		}
	})
}

func readStdinByte(buf []byte) (int, error) {
	return os.Stdin.Read(buf[:1])
}

func dangerActionText(name string) string {
	switch name {
	case "write_file":
		return "写入文件"
	case "delete_file":
		return "删除文件"
	case "execute_bash":
		return "执行 shell 命令"
	default:
		return name
	}
}

func dangerTargetText(name string, args map[string]string) string {
	switch name {
	case "write_file", "delete_file":
		return args["path"]
	case "execute_bash":
		return args["command"]
	default:
		return ""
	}
}
