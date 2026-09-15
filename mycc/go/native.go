package main

/*
#cgo CFLAGS: -std=c11 -D_POSIX_C_SOURCE=200809L -I${SRCDIR}/../c
#cgo LDFLAGS: ${SRCDIR}/../.build/libdeepcode_native.a
#include <stdlib.h>
#include <fcntl.h>
#include "native.h"
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"unsafe"
)

var subAgentProcess bool

type nativeResult struct {
	Code     int
	ExitCode int
	Output   string
	Error    string
}

func convertNative(r C.dc_native_result) nativeResult {
	defer C.dc_free_result(&r)
	out := ""
	errText := ""
	if r.output != nil {
		out = C.GoString(r.output)
	}
	if r.error != nil {
		errText = C.GoString(r.error)
	}
	return nativeResult{Code: int(r.code), ExitCode: int(r.exit_code), Output: out, Error: errText}
}

func nativeReadFile(path string) nativeResult {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return convertNative(C.dc_read_file(cPath))
}

func nativeWriteFile(path, content string) nativeResult {
	cPath := C.CString(path)
	cContent := C.CString(content)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cContent))
	return convertNative(C.dc_write_file(cPath, cContent))
}

func nativeDeleteFile(path string) nativeResult {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return convertNative(C.dc_delete_file(cPath))
}

func nativeExecuteBash(command string) nativeResult {
	cCommand := C.CString(command)
	defer C.free(unsafe.Pointer(cCommand))
	return convertNative(C.dc_execute_bash(cCommand))
}

func nativeReadWorkspaceFile(path string) nativeResult {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return convertNative(C.dc_workspace_read_file(cPath))
}

func nativeWriteWorkspaceFile(path, content string) nativeResult {
	cPath := C.CString(path)
	cContent := C.CString(content)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cContent))
	return convertNative(C.dc_workspace_write_file(cPath, cContent))
}

func nativeDeleteWorkspaceFile(path string) nativeResult {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return convertNative(C.dc_workspace_delete_file(cPath))
}

func nativeOpenWorkspaceReadFile(path string) (*os.File, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	fd, err := C.dc_open_workspace_file(cPath, C.O_RDONLY)
	if fd < 0 {
		return nil, fmt.Errorf("workspace read: %w", err)
	}
	return os.NewFile(uintptr(fd), path), nil
}

func nativeExecuteBashCancelable(ctx context.Context, command string) nativeResult {
	cCommand := C.CString(command)
	defer C.free(unsafe.Pointer(cCommand))
	newGroup := C.int(1)
	if subAgentProcess {
		newGroup = 0
	}
	return nativeWithCancellation(ctx, func(fd C.int) nativeResult {
		return convertNative(C.dc_execute_bash_cancelable(cCommand, fd, newGroup))
	})
}

func nativeWithCancellation(ctx context.Context, run func(C.int) nativeResult) nativeResult {
	if err := ctx.Err(); err != nil {
		return nativeResult{Code: -1, ExitCode: 130, Error: "canceled"}
	}
	cancelRead, cancelWrite, err := os.Pipe()
	if err != nil {
		return nativeResult{Code: -1, ExitCode: -1, Error: err.Error()}
	}
	defer cancelRead.Close()
	defer cancelWrite.Close()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			_ = cancelWrite.Close()
		case <-done:
		}
	}()
	r := run(C.int(cancelRead.Fd()))
	close(done)
	<-stopped
	return r
}

func nativeRunSubAgent(exe, task, configPath, sessionPath string, perms PermissionState) nativeResult {
	cExe := C.CString(exe)
	cTask := C.CString(task)
	cConfig := C.CString(configPath)
	cSession := C.CString(sessionPath)
	defer C.free(unsafe.Pointer(cExe))
	defer C.free(unsafe.Pointer(cTask))
	defer C.free(unsafe.Pointer(cConfig))
	defer C.free(unsafe.Pointer(cSession))
	dangerNext := C.int(0)
	if perms.DangerNext {
		dangerNext = 1
	}
	veryDanger := C.int(0)
	if perms.VeryDangerEnabled {
		veryDanger = 1
	}
	return convertNative(C.dc_run_sub_agent(cExe, cTask, cConfig, cSession, dangerNext, veryDanger))
}

func nativeRunSubAgentCancelable(ctx context.Context, exe, task, configPath, sessionPath string, perms PermissionState) nativeResult {
	cExe := C.CString(exe)
	cTask := C.CString(task)
	cConfig := C.CString(configPath)
	cSession := C.CString(sessionPath)
	defer C.free(unsafe.Pointer(cExe))
	defer C.free(unsafe.Pointer(cTask))
	defer C.free(unsafe.Pointer(cConfig))
	defer C.free(unsafe.Pointer(cSession))
	dangerNext := C.int(0)
	if perms.DangerNext {
		dangerNext = 1
	}
	veryDanger := C.int(0)
	if perms.VeryDangerEnabled {
		veryDanger = 1
	}
	return nativeWithCancellation(ctx, func(fd C.int) nativeResult {
		return convertNative(C.dc_run_sub_agent_cancelable(cExe, cTask, cConfig, cSession, dangerNext, veryDanger, fd))
	})
}

func nativePromptDangerPermission(label string) nativeResult {
	cLabel := C.CString(label)
	defer C.free(unsafe.Pointer(cLabel))
	return convertNative(C.dc_prompt_danger_permission(cLabel))
}

func nativeWorkspaceFilePath(path string) nativeResult {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return convertNative(C.dc_workspace_file_path(cPath))
}

func nativeBuildChatRequestJSON(model, messagesJSON, toolsJSON string, thinking bool) nativeResult {
	cModel := C.CString(model)
	cMessages := C.CString(messagesJSON)
	cTools := C.CString(toolsJSON)
	defer C.free(unsafe.Pointer(cModel))
	defer C.free(unsafe.Pointer(cMessages))
	defer C.free(unsafe.Pointer(cTools))
	cThinking := C.int(0)
	if thinking {
		cThinking = 1
	}
	return convertNative(C.dc_build_chat_request_json(cModel, cMessages, cTools, cThinking))
}

func nativeParseSSEChunkJSON(data string) nativeResult {
	cData := C.CString(data)
	defer C.free(unsafe.Pointer(cData))
	return convertNative(C.dc_parse_sse_chunk_json(cData))
}

func nativeParseToolArgsJSON(name, rawArgs string) nativeResult {
	cName := C.CString(name)
	cRawArgs := C.CString(rawArgs)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cRawArgs))
	return convertNative(C.dc_parse_tool_args_json(cName, cRawArgs))
}

func (r nativeResult) err() error {
	if r.Code == 0 {
		return nil
	}
	if r.Error != "" {
		return fmt.Errorf("%s", r.Error)
	}
	return fmt.Errorf("native call failed with exit code %d", r.ExitCode)
}
