package main

import (
	"fmt"
	"sync"
	"time"
)

type thinkingAnimation struct {
	done        chan struct{}
	stopped     chan struct{}
	once        sync.Once
	kind        string
	label       string
	hiddenRunes int
	mu          sync.Mutex
}

type subAgentAnimation struct {
	done     chan struct{}
	stopped  chan struct{}
	once     sync.Once
	total    int
	doneN    int
	failed   int
	canceled int
	mu       sync.Mutex
}

func startThinkingAnimation() *thinkingAnimation {
	return startActivityAnimation("thinking", "推理中")
}

func startCompressAnimation() *thinkingAnimation {
	return startActivityAnimation("thinking", "压缩上下文")
}

func startToolPrepareAnimation(name string) *thinkingAnimation {
	return startActivityAnimation("tool", toolPrepareLabel(name))
}

func startToolRunAnimation(name string) *thinkingAnimation {
	return startActivityAnimation("tool", toolRunLabel(name))
}

func startSubAgentAnimation(total int) *subAgentAnimation {
	a := &subAgentAnimation{done: make(chan struct{}), stopped: make(chan struct{}), total: total}
	go func() {
		defer close(a.stopped)
		frames := []string{"◐", "◓", "◑", "◒"}
		start := time.Now()
		i := 0
		for {
			select {
			case <-a.done:
				fmt.Print("\r\033[2K")
				return
			default:
			}
			a.mu.Lock()
			finished := a.doneN + a.failed + a.canceled
			failed := a.failed
			canceled := a.canceled
			a.mu.Unlock()
			extra := ""
			if failed > 0 || canceled > 0 {
				extra = fmt.Sprintf(" · failed %d · canceled %d", failed, canceled)
			}
			fmt.Printf("\r\033[2K%s%s 子代理并行执行中... (%d/%d · %ds%s · Esc 取消)%s",
				ansiBlue,
				frames[i%len(frames)],
				finished,
				total,
				int(time.Since(start).Seconds()),
				extra,
				ansiReset,
			)
			i++
			time.Sleep(160 * time.Millisecond)
		}
	}()
	return a
}

func startActivityAnimation(kind string, label string) *thinkingAnimation {
	a := &thinkingAnimation{done: make(chan struct{}), stopped: make(chan struct{}), kind: kind, label: label}
	go func() {
		defer close(a.stopped)
		thinkingFrames := []string{"✽", "✳", "✶", "✷"}
		toolFrames := []string{"●", "○"}
		start := time.Now()
		i := 0
		for {
			elapsed := time.Since(start)
			delay := 170 * time.Millisecond
			if elapsed > 10*time.Second {
				delay = 75 * time.Millisecond
			}
			select {
			case <-a.done:
				fmt.Print("\r\033[2K")
				return
			default:
			}
			if kind == "tool" {
				frame := toolFrames[i%len(toolFrames)]
				fmt.Printf("\r\033[2K%s%s %s%s", ansiBlue, frame, a.labelWithDots(i), ansiReset)
			} else {
				frame := thinkingFrames[i%len(thinkingFrames)]
				fmt.Printf("\r\033[2K%s%s %s%s", thinkingColor(elapsed), frame, a.thinkingLabel(i, elapsed), ansiReset)
			}
			i++
			time.Sleep(delay)
		}
	}()
	return a
}

func (a *thinkingAnimation) AddHiddenRunes(n int) {
	if a == nil || n <= 0 {
		return
	}
	a.mu.Lock()
	a.hiddenRunes += n
	a.mu.Unlock()
}

func (a *thinkingAnimation) labelWithDots(frame int) string {
	a.mu.Lock()
	hidden := a.hiddenRunes
	a.mu.Unlock()
	if hidden > 0 {
		return fmt.Sprintf("%s... (接收 %d 字)", a.label, hidden)
	}
	return a.label + "..."
}

var funThinkingLabels = []string{
	"分析上下文",
	"规划下一步",
	"检查约束",
	"整理工具结果",
	"生成回复",
	"等待模型输出",
}

func (a *thinkingAnimation) thinkingLabel(frame int, elapsed time.Duration) string {
	seconds := int(elapsed.Seconds())
	labelIdx := (seconds / 3) % len(funThinkingLabels)
	label := funThinkingLabels[labelIdx]
	a.mu.Lock()
	hidden := a.hiddenRunes
	a.mu.Unlock()
	if hidden > 0 {
		return fmt.Sprintf("%s... (%ds · ↓ %d 字 · Esc 取消)", label, seconds, hidden)
	}
	return fmt.Sprintf("%s... (%ds · Esc 取消)", label, seconds)
}

func (a *thinkingAnimation) Stop() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		close(a.done)
		<-a.stopped
	})
}

func (a *subAgentAnimation) Mark(status string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch status {
	case "done":
		a.doneN++
	case "failed":
		a.failed++
	case "canceled":
		a.canceled++
	}
}

func (a *subAgentAnimation) Stop() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		close(a.done)
		<-a.stopped
	})
}

func thinkingColor(elapsed time.Duration) string {
	if elapsed <= 10*time.Second {
		return ansiBlue
	}
	if elapsed >= 30*time.Second {
		return "\033[38;2;255;80;80m"
	}
	ratio := float64(elapsed-10*time.Second) / float64(20*time.Second)
	red := 64 + int(191*ratio)
	green := 156 - int(76*ratio)
	blue := 255 - int(175*ratio)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", red, green, blue)
}

func toolPrepareLabel(name string) string {
	switch name {
	case "get_cwd":
		return "查看当前目录"
	case "read_file":
		return "准备读取文件"
	case "write_file":
		return "准备写入文件"
	case "delete_file":
		return "准备删除文件"
	case "execute_bash":
		return "准备执行命令"
	default:
		return "准备调用工具"
	}
}

func toolRunLabel(name string) string {
	switch name {
	case "get_cwd":
		return "查看当前目录"
	case "read_file":
		return "读取文件"
	case "write_file":
		return "写入文件"
	case "delete_file":
		return "删除文件"
	case "execute_bash":
		return "执行命令"
	default:
		return "运行工具"
	}
}
