package middleware

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundElicitationQuestionKeepsAShortQuestionWhole(t *testing.T) {
	const question = "Which database should the migration target?"
	got, truncated := BoundElicitationQuestion(question)
	if got != question || truncated {
		t.Fatalf("domanda corta: got=%q truncated=%v, vuole il testo intero e non marcato", got, truncated)
	}
}

func TestBoundElicitationQuestionCollapsesWhitespaceIntoOneLine(t *testing.T) {
	got, truncated := BoundElicitationQuestion("Approve this plan?\n\n\tStep 1: migrate\nStep 2: verify  ")
	const want = "Approve this plan? Step 1: migrate Step 2: verify"
	if got != want || truncated {
		t.Fatalf("righe multiple: got=%q truncated=%v, vuole %q in una riga", got, truncated, want)
	}
	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("la domanda conserva un a capo: %q", got)
	}
}

func TestBoundElicitationQuestionMarksNothingWhenThereIsNothingToCut(t *testing.T) {
	for _, message := range []string{"", "   ", "\n\t "} {
		got, truncated := BoundElicitationQuestion(message)
		if got != "" || truncated {
			t.Fatalf("messaggio %q: got=%q truncated=%v, vuole vuoto e non marcato", message, got, truncated)
		}
	}
}

func TestBoundElicitationQuestionMarksTheCutAtTheDeclaredThreshold(t *testing.T) {
	exact := strings.Repeat("a", ElicitationQuestionMaxRunes)
	if got, truncated := BoundElicitationQuestion(exact); got != exact || truncated {
		t.Fatalf("esattamente alla soglia: truncated=%v, vuole non marcato", truncated)
	}

	over := exact + "b"
	got, truncated := BoundElicitationQuestion(over)
	if !truncated {
		t.Fatal("una domanda oltre la soglia deve essere marcata come troncata")
	}
	if runes := utf8.RuneCountInString(got); runes != ElicitationQuestionMaxRunes {
		t.Fatalf("troncamento a %d rune, vuole %d", runes, ElicitationQuestionMaxRunes)
	}
	if strings.ContainsRune(got, 'b') {
		t.Fatal("la coda oltre la soglia non deve sopravvivere")
	}
}

func TestBoundElicitationQuestionNeverSplitsAMultiByteCharacter(t *testing.T) {
	for name, unit := range map[string]string{"accentata": "à", "emoji": "🙂", "cjk": "漢"} {
		t.Run(name, func(t *testing.T) {
			got, truncated := BoundElicitationQuestion(strings.Repeat(unit, ElicitationQuestionMaxRunes*2))
			if !truncated {
				t.Fatal("una domanda oltre la soglia deve essere marcata come troncata")
			}
			if !utf8.ValidString(got) {
				t.Fatalf("il troncamento ha spezzato un carattere: %q", got)
			}
			if runes := utf8.RuneCountInString(got); runes != ElicitationQuestionMaxRunes {
				t.Fatalf("troncamento a %d rune, vuole %d", runes, ElicitationQuestionMaxRunes)
			}
		})
	}
}

// TestBoundElicitationQuestionDoesNotEchoTheTail is the property that matters
// for a redacted surface: the bounded question is a prefix of what a person was
// asked, never a window onto whatever followed it. If a consumer ever saw text
// past the threshold, the "bounded field" would be a content channel wearing a
// summary's name.
func TestBoundElicitationQuestionDoesNotEchoTheTail(t *testing.T) {
	const sentinel = "TRANSCRIPT-ECHO-SENTINEL"
	got, truncated := BoundElicitationQuestion(strings.Repeat("x", ElicitationQuestionMaxRunes*10) + sentinel)
	if !truncated {
		t.Fatal("il testo oltre la soglia deve risultare troncato")
	}
	if strings.Contains(got, sentinel) {
		t.Fatalf("la coda oltre la soglia è finita nell'involucro: %q", got)
	}
}
