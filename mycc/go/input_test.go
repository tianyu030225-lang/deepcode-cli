package main

import "testing"

func TestPromptInputLinesWrapsLongInput(t *testing.T) {
	lines := promptInputLines("abcdef", 5)
	if len(lines) != 2 || lines[0] != "abc" || lines[1] != "def" {
		t.Fatalf("wrapped lines = %#v", lines)
	}
}

func TestPromptInputLinesKeepsManualNewline(t *testing.T) {
	lines := promptInputLines("第一行\n第二行", 20)
	if len(lines) != 2 || lines[0] != "第一行" || lines[1] != "第二行" {
		t.Fatalf("manual newline lines = %#v", lines)
	}
}
