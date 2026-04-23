package telegram

import (
	"strings"
	"testing"
)

func TestSplitTelegramMessageRespectsLimit(t *testing.T) {
	input := strings.Repeat("a", telegramSafeMessageLimit+20)
	chunks := splitTelegramMessage(input, telegramSafeMessageLimit)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > telegramSafeMessageLimit {
			t.Fatalf("chunk exceeds telegram limit: %d", len([]rune(chunk)))
		}
	}
}

func TestSplitTelegramMessagePrefersNewline(t *testing.T) {
	input := strings.Repeat("a", 40) + "\n" + strings.Repeat("b", 40)
	chunks := splitTelegramMessage(input, 50)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if strings.Contains(chunks[0], "b") {
		t.Fatalf("expected first chunk to split on newline, got %q", chunks[0])
	}
}
