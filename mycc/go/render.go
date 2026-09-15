package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type replyRenderer struct {
	mode        string
	colorIdx    int
	inCodeFence bool
}

func newReplyRenderer(cfg Config) *replyRenderer {
	return &replyRenderer{mode: cfg.DisplayMode}
}

func (r *replyRenderer) Write(text string) {
	if r == nil {
		fmt.Print(text)
		return
	}
	cleaned := r.cleanMarkdown(text)
	r.writeCleaned(cleaned)
}

func (r *replyRenderer) writeCleaned(cleaned string) {
	if r == nil {
		fmt.Print(cleaned)
		return
	}
	if r.mode != displayColorful {
		fmt.Print(cleaned)
		return
	}
	color := r.nextColor(cleaned)
	if color == "" {
		fmt.Print(cleaned)
		return
	}
	fmt.Print(color + cleaned + ansiReset)
}

type streamingReplyRenderer struct {
	base         *replyRenderer
	done         chan struct{}
	stopped      chan struct{}
	once         sync.Once
	mu           sync.Mutex
	segments     []streamSegment
	frame        string
	activeColor  string
	lineCol      int
	hasIndicator bool
}

type streamSegment struct {
	text  string
	color string
}

func startStreamingReplyRenderer(cfg Config) *streamingReplyRenderer {
	r := &streamingReplyRenderer{
		base:    newReplyRenderer(cfg),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
		frame:   "·",
	}
	go r.loop()
	return r
}

func (r *streamingReplyRenderer) Write(text string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	cleaned := ""
	if r.base != nil {
		cleaned = r.base.cleanMarkdown(text)
	} else {
		cleaned = text
	}
	if cleaned == "" {
		r.mu.Unlock()
		return
	}
	color := ""
	if r.base != nil && r.base.mode == displayColorful {
		color = r.base.nextColor(cleaned)
	}
	r.segments = append(r.segments, streamSegment{text: cleaned, color: color})
	r.mu.Unlock()
}

func (r *streamingReplyRenderer) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.done)
		<-r.stopped
		r.finishOutput()
	})
}

func (r *streamingReplyRenderer) loop() {
	defer close(r.stopped)
	ticker := time.NewTicker(14 * time.Millisecond)
	defer ticker.Stop()
	frames := []string{"·", "▫", "◇", "◆", "■", "◆", "◇", "▫"}
	i := 0
	lastFrame := time.Now()
	done := r.done
	for {
		select {
		case <-done:
			done = nil
		case now := <-ticker.C:
			if r.printNext() {
				continue
			}
			if done == nil {
				return
			}
			if now.Sub(lastFrame) >= 140*time.Millisecond {
				r.drawIndicator(frames[i%len(frames)])
				i++
				lastFrame = now
			}
		}
	}
}

func (r *streamingReplyRenderer) printNext() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	unit, color, ok := r.nextUnitLocked()
	if !ok {
		return false
	}
	r.clearIndicatorLocked()
	if color != r.activeColor {
		if r.activeColor != "" {
			fmt.Print(ansiReset)
		}
		if color != "" {
			fmt.Print(color)
		}
		r.activeColor = color
	}
	fmt.Print(unit)
	r.advanceCursor(unit)
	return true
}

func (r *streamingReplyRenderer) nextUnitLocked() (string, string, bool) {
	for len(r.segments) > 0 {
		seg := &r.segments[0]
		if seg.text == "" {
			r.segments = r.segments[1:]
			continue
		}
		_, size := utf8.DecodeRuneInString(seg.text)
		if size <= 0 {
			size = 1
		}
		unit := seg.text[:size]
		color := seg.color
		seg.text = seg.text[size:]
		if seg.text == "" {
			r.segments = r.segments[1:]
		}
		return unit, color, true
	}
	return "", "", false
}

func (r *streamingReplyRenderer) drawIndicator(frame string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drawIndicatorLocked(frame)
}

func (r *streamingReplyRenderer) drawIndicatorLocked(frame string) {
	// The persistent input frame uses an ANSI scroll region. Drawing a
	// transient indicator with a newline scrolls blank lines into the reply area,
	// so streaming output deliberately avoids an idle indicator.
}

func (r *streamingReplyRenderer) clearIndicator() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearIndicatorLocked()
}

func (r *streamingReplyRenderer) finishOutput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearIndicatorLocked()
	if r.activeColor != "" {
		fmt.Print(ansiReset)
		r.activeColor = ""
	}
}

func (r *streamingReplyRenderer) clearIndicatorLocked() {
	r.hasIndicator = false
}

func (r *streamingReplyRenderer) advanceCursor(text string) {
	width := terminalWidth()
	if width < 1 {
		width = 80
	}
	for _, ch := range text {
		switch ch {
		case '\n', '\r':
			r.lineCol = 0
		default:
			w := 1
			if isWideRune(ch) {
				w = 2
			}
			r.lineCol += w
			if r.lineCol >= width {
				r.lineCol %= width
			}
		}
	}
}

func (r *replyRenderer) nextColor(text string) string {
	if strings.TrimSpace(text) == "" || strings.Contains(text, "```") {
		return ""
	}
	if strings.Contains(text, "⚠️") || strings.Contains(text, "错误") || strings.Contains(text, "失败") {
		return "\033[38;2;255;95;95m"
	}
	if strings.Contains(text, "✅") || strings.Contains(text, "完成") || strings.Contains(text, "成功") {
		return "\033[38;2;80;220;150m"
	}
	palette := []string{
		"\033[38;2;125;190;255m",
		"\033[38;2;120;230;190m",
		"\033[38;2;255;205;120m",
		"\033[38;2;210;165;255m",
	}
	color := palette[r.colorIdx%len(palette)]
	r.colorIdx++
	return color
}

func (r *replyRenderer) cleanMarkdown(text string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			r.inCodeFence = !r.inCodeFence
			// keep the fence line as-is
			continue
		}
		if r.inCodeFence {
			// inside code block: don't touch anything
			continue
		}
		// outside code fence: strip ** and convert headings
		line = strings.ReplaceAll(line, "**", "")
		trimmed = strings.TrimLeft(line, " \t")
		prefixLen := len(line) - len(trimmed)
		switch {
		case strings.HasPrefix(trimmed, "### "):
			lines[i] = line[:prefixLen] + "▌ " + strings.TrimPrefix(trimmed, "### ")
		case strings.HasPrefix(trimmed, "## "):
			lines[i] = line[:prefixLen] + "▌ " + strings.TrimPrefix(trimmed, "## ")
		case strings.HasPrefix(trimmed, "# "):
			lines[i] = line[:prefixLen] + "▌ " + strings.TrimPrefix(trimmed, "# ")
		default:
			lines[i] = line
		}
	}
	return strings.Join(lines, "")
}
