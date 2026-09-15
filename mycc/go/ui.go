package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

const (
	ansiBlue  = "\033[38;2;64;156;255m"
	ansiDim   = "\033[2m"
	ansiRed   = "\033[31m"
	ansiReset = "\033[0m"
)

func hideCursor() {
	fmt.Print("\033[?25l")
}

func showCursor() {
	fmt.Print("\033[?25h")
}

type termios struct {
	Iflag  uint32
	Oflag  uint32
	Cflag  uint32
	Lflag  uint32
	Line   uint8
	Cc     [32]uint8
	Ispeed uint32
	Ospeed uint32
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func terminalWidth() int {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.Col == 0 {
		return 120
	}
	if ws.Col < 60 {
		return 60
	}
	return int(ws.Col)
}

func ioctlGetTermios(fd int) (termios, error) {
	var t termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		return t, errno
	}
	return t, nil
}

func ioctlSetTermios(fd int, t termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		return errno
	}
	return nil
}

func withRawMode(fn func() error) error {
	return withRawModeConfig(1, 0, fn)
}

func withRawModeTimed(fn func() error) error {
	return withRawModeConfig(0, 1, fn)
}

func withRawModeConfig(vmin uint8, vtime uint8, fn func() error) error {
	fd := int(os.Stdin.Fd())
	old, err := ioctlGetTermios(fd)
	if err != nil {
		return fn()
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = vmin
	raw.Cc[syscall.VTIME] = vtime
	if err := ioctlSetTermios(fd, raw); err != nil {
		return fn()
	}
	defer ioctlSetTermios(fd, old)
	return fn()
}

type escWatcher struct {
	done     chan struct{}
	stopped  chan struct{}
	canceled chan struct{}
	once     sync.Once
	stopOnce sync.Once
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func showStartupAnimation(force bool) {
	clearScreen()
	lines := []string{
		"     ██████╗ ███████╗███████╗██████╗ ███████╗███████╗███████╗██╗  ██╗",
		"     ██╔══██╗██╔════╝██╔════╝██╔══██╗██╔════╝██╔════╝██╔════╝██║ ██╔╝",
		"     ██║  ██║█████╗  █████╗  ██████╔╝███████╗█████╗  █████╗  █████╔╝ ",
		"     ██║  ██║██╔══╝  ██╔══╝  ██╔═══╝ ╚════██║██╔══╝  ██╔══╝  ██╔═██╗ ",
		"     ██████╔╝███████╗███████╗██║     ███████║███████╗███████╗██║  ██╗",
		"     ╚═════╝ ╚══════╝╚══════╝╚═╝     ╚══════╝╚══════╝╚══════╝╚═╝  ╚═╝",
	}
	fmt.Print(ansiBlue)
	for _, line := range lines {
		for len(line) > 0 {
			r, size := utf8.DecodeRuneInString(line)
			fmt.Print(string(r))
			line = line[size:]
			if force {
				time.Sleep(2 * time.Millisecond)
			}
		}
		fmt.Println()
	}
	fmt.Print(ansiReset)
	time.Sleep(350 * time.Millisecond)
}

func promptLine(label string, secret bool) (string, error) {
	fmt.Print(ansiBlue + label + ansiReset)
	if secret {
		return readSecretLine()
	}
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	return strings.TrimSpace(text), err
}

func readSecretLine() (string, error) {
	var b strings.Builder
	err := withRawMode(func() error {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			ch := buf[0]
			switch ch {
			case '\r', '\n':
				fmt.Println()
				return nil
			case 3:
				return fmt.Errorf("取消输入")
			case 127, 8:
				s := b.String()
				if len(s) > 0 {
					_, size := utf8.DecodeLastRuneInString(s)
					b.Reset()
					b.WriteString(s[:len(s)-size])
					fmt.Print("\b \b")
				}
			default:
				b.WriteByte(ch)
				fmt.Print("*")
			}
		}
	})
	return strings.TrimSpace(b.String()), err
}

func chooseMenu(title string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, fmt.Errorf("没有可选项")
	}
	selected := 0
	render := func() {
		fmt.Print("\033[?25l")
		clearScreen()
		fmt.Println(ansiBlue + title + ansiReset)
		for i, opt := range options {
			if i == selected {
				fmt.Println(ansiBlue + "  › " + opt + ansiReset)
			} else {
				fmt.Println("    " + opt)
			}
		}
	}
	err := withRawMode(func() error {
		render()
		buf := make([]byte, 3)
		for {
			n, err := os.Stdin.Read(buf[:1])
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			switch buf[0] {
			case '\r', '\n':
				fmt.Print("\033[?25h")
				return nil
			case 3:
				fmt.Print("\033[?25h")
				return fmt.Errorf("取消选择")
			case 27:
				n, err := os.Stdin.Read(buf[1:2])
				if err != nil {
					return err
				}
				if n == 0 {
					continue
				}
				n, err = os.Stdin.Read(buf[2:3])
				if err != nil {
					return err
				}
				if n == 0 {
					continue
				}
				if buf[1] == '[' {
					if buf[2] == 'A' && selected > 0 {
						selected--
					}
					if buf[2] == 'B' && selected < len(options)-1 {
						selected++
					}
					render()
				}
			}
		}
	})
	fmt.Print("\033[?25h")
	clearScreen()
	return selected, err
}

func typePrint(text string, delay time.Duration) {
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		fmt.Print(string(r))
		text = text[size:]
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

func statusLine(text string) {
	fmt.Println(ansiDim + text + ansiReset)
}

func dangerLine(text string) {
	fmt.Println(ansiRed + text + ansiReset)
}

func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		if r == '\n' || r == '\r' {
			continue
		}
		if isWideRune(r) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6)
}

func fitText(s string, width int) string {
	if displayWidth(s) <= width {
		return s + strings.Repeat(" ", width-displayWidth(s))
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := 1
		if isWideRune(r) {
			w = 2
		}
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + strings.Repeat(" ", width-displayWidth(b.String()))
}

func centerText(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return fitText(s, width)
	}
	left := (width - w) / 2
	right := width - w - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func repeatLine(ch string, width int) string {
	return strings.Repeat(ch, width)
}

func padDisplayRight(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return fitText(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func cwdDisplay() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "当前目录"
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, relErr := filepath.Rel(home, cwd); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			if rel == "." {
				return "~"
			}
			return "~/" + filepath.ToSlash(rel)
		}
	}
	if base := filepath.Base(cwd); base != "." && base != string(os.PathSeparator) {
		return base
	}
	return cwd
}

func workspaceInstructionFileCount() int {
	entries, err := os.ReadDir(".")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "CLAUDE.md") {
			count++
		}
	}
	return count
}

func contextPercent(s Session) int {
	chars := 0
	for _, msg := range s.Messages {
		chars += len([]rune(msg.Content))
		chars += len([]rune(msg.ReasoningContent))
	}
	if chars == 0 {
		return 0
	}
	pct := chars * 100 / 200000
	if pct > 100 {
		pct = 100
	}
	if pct < 1 {
		pct = 1
	}
	return pct
}

func contextBar(percent int) string {
	filled := percent / 10
	if percent > 0 && filled == 0 {
		filled = 1
	}
	if filled > 10 {
		filled = 10
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 10-filled)
}

func renderWelcomePanel(cfg Config, s Session) {
	outerWidth := promptFrameWidth()
	innerWidth := outerWidth - 2
	leftWidth := 43
	rightWidth := innerWidth - leftWidth - 3
	if rightWidth < 30 {
		leftWidth = innerWidth/2 - 2
		rightWidth = innerWidth - leftWidth - 3
	}
	if leftWidth < 22 || rightWidth < 22 {
		title := "─── Deepcode v0.1.0 "
		fmt.Println(ansiBlue + "╭" + title + repeatLine("─", innerWidth-displayWidth(title)) + "╮" + ansiReset)
		fmt.Printf("│%s│\n", centerText("Welcome back!", innerWidth))
		fmt.Printf("│%s│\n", ansiBlue+centerText("▐▛███▜▌", innerWidth)+ansiReset)
		fmt.Printf("│%s│\n", centerText(sessionTitleForDisplay(s), innerWidth))
		fmt.Printf("│%s│\n", centerText(currentSessionModel(cfg, s)+" · "+cfg.Provider, innerWidth))
		fmt.Println(ansiBlue + "╰" + repeatLine("─", innerWidth) + "╯" + ansiReset)
		fmt.Println()
		return
	}

	title := "─── Deepcode v0.1.0 "
	fmt.Println(ansiBlue + "╭" + title + repeatLine("─", innerWidth-displayWidth(title)) + "╮" + ansiReset)

	left := []string{
		"",
		"Welcome back!",
		"",
		"▐▛███▜▌",
		"▝▜█████▛▘",
		"▘▘ ▝▝",
		"",
		sessionTitleForDisplay(s),
		currentSessionModel(cfg, s) + " · " + providerLabel(cfg.Provider),
		cwdDisplay(),
	}
	right := []string{
		"Tips for getting started",
		"Run / 查看全部命令",
		"────────────────────────────────",
		"What's new",
		"输入 /rename 重命名当前会话",
		"输入 /思考 开关 DeepSeek thinking",
		"输入 /恢复 继续本地历史对话",
		"",
		"",
		"",
	}
	for i := 0; i < len(left); i++ {
		l := centerText(left[i], leftWidth)
		if i >= 3 && i <= 5 {
			l = ansiBlue + l + ansiReset
		}
		r := padDisplayRight(right[i], rightWidth)
		fmt.Printf("│%s │ %s│\n", l, r)
	}
	fmt.Println(ansiBlue + "╰" + repeatLine("─", innerWidth) + "╯" + ansiReset)
	fmt.Println()
}

func renderPromptFrame(cfg Config, s Session, input string, suggestions []commandInfo, placeCursor bool) {
	if placeCursor {
		hideCursor()
		defer showCursor()
	}
	percent := contextPercent(s)
	width := promptFrameWidth()
	fmt.Println(repeatLine("─", width))
	fmt.Println(ansiBlue + "❯ " + ansiReset + input)
	renderPromptLowerLines(cfg, s, suggestions, width, percent)
	if placeCursor {
		linesAfterPrompt := 1 + promptLowerLineCount(suggestions)
		col := displayWidth("❯ ") + displayWidth(input)
		fmt.Printf("\033[%dA\r\033[%dC", linesAfterPrompt, col)
	}
}

func redrawPromptLower(cfg Config, s Session, suggestions []commandInfo, input string) {
	width := promptFrameWidth()
	percent := contextPercent(s)
	lineCount := promptLowerLineCount(suggestions)
	fmt.Print("\033[1B\r\033[J")
	renderPromptLowerLines(cfg, s, suggestions, width, percent)
	col := displayWidth("❯ ") + displayWidth(input)
	fmt.Printf("\033[%dA\r\033[%dC", lineCount+1, col)
}

func renderPromptLowerLines(cfg Config, s Session, suggestions []commandInfo, width int, percent int) {
	fmt.Println(repeatLine("─", width))
	for _, item := range suggestions {
		left := fitText(item.Name, 32)
		right := fitText(item.Description, width-33)
		fmt.Printf("%s %s\n", left, right)
	}
	model := currentSessionModel(cfg, s)
	dir := cwdDisplay()
	if len([]rune(dir)) > 48 {
		runes := []rune(dir)
		dir = "..." + string(runes[len(runes)-45:])
	}
	tokenInfo := ""
	if s.TotalPromptTokens > 0 || s.TotalCompletionTokens > 0 {
		tokenInfo = fmt.Sprintf(" · ↑%d ↓%d tok", s.TotalPromptTokens, s.TotalCompletionTokens)
	}
	top := fmt.Sprintf("  [%s] │ %s", model, dir)
	if title := sessionTitleForDisplay(s); title != "" {
		top += " · " + title
	}
	if cfg.Provider != "" {
		top += "  ◈ " + cfg.Provider
	}
	if s.ThinkingEnabled {
		top += " · /思考"
	}
	fmt.Println(fitText(top+tokenInfo, width))
	fmt.Println(fitText(fmt.Sprintf("  上下文 %s %d%%", contextBar(percent), percent), width))
	if count := workspaceInstructionFileCount(); count > 0 {
		fmt.Println(fitText(fmt.Sprintf("  %d CLAUDE.md", count), width))
	}
}

func promptLowerLineCount(suggestions []commandInfo) int {
	return promptLowerLineCountForSuggestions(len(suggestions))
}

func promptLowerLineCountForSuggestions(suggestionCount int) int {
	count := 3 + suggestionCount
	if workspaceInstructionFileCount() > 0 {
		count++
	}
	return count
}

func providerLabel(provider string) string {
	switch provider {
	case providerDeepSeek:
		return "API Usage Billing"
	case providerCustom:
		return "custom"
	default:
		return provider
	}
}

func promptFrameWidth() int {
	width := terminalWidth()
	if width > 120 {
		width = 120
	}
	if width < 60 {
		width = 60
	}
	return width
}

func currentSessionModel(cfg Config, s Session) string {
	if s.Model != "" {
		return s.Model
	}
	if cfg.DefaultModel != "" {
		return cfg.DefaultModel
	}
	return defaultModel
}

func sessionTitleForDisplay(s Session) string {
	title := strings.TrimSpace(s.Title)
	if title == "" {
		return "新对话"
	}
	if len([]rune(title)) > 30 {
		runes := []rune(title)
		return string(runes[:30]) + "..."
	}
	return title
}
