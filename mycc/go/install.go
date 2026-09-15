package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func isInstalledBinary(paths Paths) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, _ = filepath.EvalSymlinks(exe)
	target, _ := filepath.EvalSymlinks(paths.BinPath)
	return exe == target
}

func runInstall(paths Paths) error {
	runInstallProgress("Deepcode installer")
	if err := ensureDataDirs(paths); err != nil {
		return err
	}
	if err := copySelf(paths.BinPath); err != nil {
		return err
	}
	pathUpdated, err := ensureLocalBinInShellPATH(paths)
	if err != nil {
		fmt.Println(ansiRed + "PATH 自动配置失败：" + ansiReset + err.Error())
	}
	fmt.Println(ansiBlue + "安装完成：" + ansiReset + paths.BinPath)
	if pathUpdated {
		fmt.Println("已把 ~/.local/bin 加入 shell 配置。请重新打开终端，或运行：")
		fmt.Println("  export PATH=\"$HOME/.local/bin:$PATH\"")
	} else {
		fmt.Println("如果 shell 找不到 deepcode，请运行：export PATH=\"$HOME/.local/bin:$PATH\"")
	}
	return nil
}

func runInstallProgress(title string) {
	fmt.Print("\033[?25l") // hide cursor
	defer fmt.Print("\033[?25h")

	clearScreen()

	logo := []string{
		"                   \u2590\u259b\u2588\u2588\u2588\u259c\u258c                   ",
		"                  \u259d\u259c\u2588\u2588\u2588\u2588\u2588\u259b\u2598                  ",
		"                    \u2598\u2598 \u259d\u259d                    ",
	}
	fmt.Println()
	for _, line := range logo {
		fmt.Printf("  %s%s%s\n", ansiBlue, line, ansiReset)
	}
	fmt.Println()
	fmt.Printf("  %s%s%s\n", ansiBlue, title, ansiReset)
	fmt.Println()

	installSteps := []string{
		"初始化安装环境",
		"复制程序文件",
		"配置系统路径",
		"注册快捷命令",
		"安装完成",
	}

	type keyframe struct {
		percent  int
		stepDone int // index of step now complete (-1 = none)
		delay    time.Duration
	}
	frames := []keyframe{
		{5, -1, 250 * time.Millisecond},
		{13, 0, 400 * time.Millisecond},
		{26, -1, 450 * time.Millisecond},
		{40, 1, 600 * time.Millisecond},
		{54, -1, 650 * time.Millisecond},
		{67, 2, 600 * time.Millisecond},
		{78, -1, 500 * time.Millisecond},
		{88, 3, 650 * time.Millisecond},
		{93, -1, 400 * time.Millisecond},
		{97, -1, 300 * time.Millisecond},
		{100, 4, 250 * time.Millisecond},
	}

	spinFrames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	// animated section: bar line + blank + 5 step lines = 7 lines
	animLines := 2 + len(installSteps)
	completedSteps := 0
	currentStep := 0
	spinIdx := 0

	printFrame := func(percent int) {
		fmt.Printf("  %s%s %3d%%%s\n", ansiBlue, progressBar(percent), percent, ansiReset)
		fmt.Println()
		for i, step := range installSteps {
			switch {
			case i < completedSteps:
				fmt.Printf("  %s\u2713%s %s\n", ansiBlue, ansiReset, step)
			case i == currentStep:
				spin := spinFrames[spinIdx%len(spinFrames)]
				fmt.Printf("  %s%s%s %s\n", ansiBlue, spin, ansiReset, step)
			default:
				fmt.Printf("  %s\u00b7%s %s\n", ansiDim, ansiReset, step)
			}
		}
	}

	printFrame(0)

	for _, f := range frames {
		time.Sleep(f.delay)
		spinIdx++
		if f.stepDone >= 0 {
			completedSteps = f.stepDone + 1
			if completedSteps < len(installSteps) {
				currentStep = completedSteps
			}
		}
		fmt.Printf("\033[%dA", animLines)
		printFrame(f.percent)
	}
	fmt.Println()
}

func progressBar(percent int) string {
	width := 28
	filled := percent * width / 100
	if percent > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", width-filled) + "]"
}

func copySelf(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func ensureLocalBinInShellPATH(paths Paths) (bool, error) {
	if pathContains(paths.BinDir) {
		return false, nil
	}
	for _, file := range shellPathFiles(paths) {
		data, err := os.ReadFile(file)
		if err == nil && strings.Contains(string(data), ".local/bin") {
			return false, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	file := preferredShellPathFile(paths)
	line := "\n# Added by deepcode\nexport PATH=\"$HOME/.local/bin:$PATH\"\n"
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func shellPathFiles(paths Paths) []string {
	return []string{
		filepath.Join(paths.Home, ".bashrc"),
		filepath.Join(paths.Home, ".zshrc"),
		filepath.Join(paths.Home, ".profile"),
	}
}

func preferredShellPathFile(paths Paths) string {
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		return filepath.Join(paths.Home, ".bashrc")
	case "zsh":
		return filepath.Join(paths.Home, ".zshrc")
	default:
		return filepath.Join(paths.Home, ".profile")
	}
}

func pathContains(dir string) bool {
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if item == dir {
			return true
		}
	}
	return false
}
