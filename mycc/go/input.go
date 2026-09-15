package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

func readInputLine(app *App) (string, error) {
	var input string
	err := withRawMode(func() error {
		enableExtendedKeyboard()
		defer disableExtendedKeyboard()
		renderPromptFrame(app.Config, app.Session, input, nil, true)
		renderState := inputRenderState{rows: 1}
		showingLowerSuggestions := false
		buf := make([]byte, 8)
		for {
			n, err := os.Stdin.Read(buf[:1])
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			ch := buf[0]
			switch ch {
			case '\r':
				renderState.finish(input)
				return nil
			case '\n':
				input += "\n"
				showingLowerSuggestions = renderState.redraw(app, input, showingLowerSuggestions)
			case 3:
				return fmt.Errorf("取消输入")
			case 9:
				input = completeSlash(input)
				showingLowerSuggestions = renderState.redraw(app, input, showingLowerSuggestions)
			case 127, 8:
				input = trimLastRune(input)
				showingLowerSuggestions = renderState.redraw(app, input, showingLowerSuggestions)
			case 27:
				switch readEscapeSequence(buf) {
				case "newline":
					input += "\n"
					showingLowerSuggestions = renderState.redraw(app, input, showingLowerSuggestions)
				case "escape":
					return fmt.Errorf("取消输入")
				}
			default:
				if ch < 0x80 {
					input += string(ch)
				} else {
					need := utf8ByteLen(ch)
					tmp := []byte{ch}
					for len(tmp) < need {
						n, err := readStdinByte(buf)
						if err != nil {
							return err
						}
						if n == 0 {
							continue
						}
						tmp = append(tmp, buf[0])
					}
					input += string(tmp)
				}
				showingLowerSuggestions = renderState.redraw(app, input, showingLowerSuggestions)
			}
		}
	})
	return strings.TrimSpace(input), err
}

func enableExtendedKeyboard() {
	fmt.Print("\033[>4;2m")
}

func disableExtendedKeyboard() {
	fmt.Print("\033[>4;0m")
}

func readEscapeSequence(buf []byte) string {
	if !stdinByteAvailable(50 * time.Millisecond) {
		return "escape"
	}
	n, err := readStdinByte(buf)
	if err != nil || n == 0 {
		return "escape"
	}
	if buf[0] == '\r' || buf[0] == '\n' {
		return "newline"
	}
	if buf[0] != '[' {
		return ""
	}
	seq := []byte{'\033', '['}
	for len(seq) < 12 {
		if !stdinByteAvailable(50 * time.Millisecond) {
			return ""
		}
		n, err := readStdinByte(buf)
		if err != nil || n == 0 {
			return ""
		}
		seq = append(seq, buf[0])
		if (buf[0] >= 'A' && buf[0] <= 'Z') || (buf[0] >= 'a' && buf[0] <= 'z') || buf[0] == '~' {
			break
		}
	}
	switch string(seq) {
	case "\033[13;2u", "\033[13;2~", "\033[27;2;13~":
		return "newline"
	default:
		return ""
	}
}

func stdinByteAvailable(timeout time.Duration) bool {
	fd := int(os.Stdin.Fd())
	var readfds syscall.FdSet
	fdSet(fd, &readfds)
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())
	n, err := syscall.Select(fd+1, &readfds, nil, nil, &tv)
	return err == nil && n > 0
}

func fdSet(fd int, set *syscall.FdSet) {
	set.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
}

func utf8ByteLen(first byte) int {
	switch {
	case first&0xE0 == 0xC0:
		return 2
	case first&0xF0 == 0xE0:
		return 3
	case first&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
}

type inputRenderState struct {
	rows int
}

func (s *inputRenderState) redraw(app *App, input string, showingLowerSuggestions bool) bool {
	hideCursor()
	defer showCursor()
	var suggestions []commandInfo
	if strings.HasPrefix(input, "/") {
		suggestions = visibleCommandMatches(input)
	}
	width := promptFrameWidth()
	s.moveToPromptStart()
	fmt.Print("\033[J")
	printPromptInput(input, width)
	renderPromptLowerLines(app.Config, app.Session, suggestions, width, contextPercent(app.Session))
	s.rows = len(promptInputLines(input, width))
	s.placeCursor(input, len(suggestions), width)
	return len(suggestions) > 0
}

func (s *inputRenderState) finish(input string) {
	width := promptFrameWidth()
	s.moveToPromptStart()
	fmt.Print("\033[J")
	printPromptInput(input, width)
}

func (s *inputRenderState) moveToPromptStart() {
	if s.rows > 1 {
		fmt.Printf("\033[%dA", s.rows-1)
	}
	fmt.Print("\r")
}

func (s *inputRenderState) placeCursor(input string, suggestionCount int, width int) {
	lines := promptInputLines(input, width)
	last := ""
	if len(lines) > 0 {
		last = lines[len(lines)-1]
	}
	up := 1 + promptLowerLineCountForSuggestions(suggestionCount)
	col := displayWidth("❯ ") + displayWidth(last)
	fmt.Printf("\033[%dA\r", up)
	if col > 0 {
		fmt.Printf("\033[%dC", col)
	}
}

func printPromptInput(input string, width int) {
	lines := promptInputLines(input, width)
	for i, line := range lines {
		if i == 0 {
			fmt.Print(ansiBlue + "❯ " + ansiReset + line)
		} else {
			fmt.Print("  " + line)
		}
		fmt.Println()
	}
}

func promptInputLines(input string, width int) []string {
	prefixWidth := displayWidth("❯ ")
	available := width - prefixWidth
	if available < 1 {
		available = 1
	}
	parts := strings.Split(input, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, wrapPromptInputLine(part, available)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapPromptInputLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	var lines []string
	var b strings.Builder
	used := 0
	for _, r := range line {
		w := 1
		if isWideRune(r) {
			w = 2
		}
		if used > 0 && used+w > width {
			lines = append(lines, b.String())
			b.Reset()
			used = 0
		}
		b.WriteRune(r)
		used += w
	}
	lines = append(lines, b.String())
	return lines
}

func completeSlash(input string) string {
	if !strings.HasPrefix(input, "/") {
		return input
	}
	matches := visibleCommandMatches(input)
	if len(matches) == 0 {
		return input
	}
	return matches[0].Name
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(s)
	if size <= 0 {
		return ""
	}
	return s[:len(s)-size]
}
