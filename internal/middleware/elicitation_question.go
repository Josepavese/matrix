package middleware

import "strings"

// ElicitationQuestionMaxRunes bounds how much of a question a summary envelope
// may carry.
//
// The bound is declared, not measured, and it is declared because a question is
// user content: the notification stream is an envelope that says what a run is
// waiting for, not a channel that carries what the user typed. A question that
// does not fit is not lost - the SDK reads it from the elicitation itself - so
// the only thing this bound can damage is a summary that was already too long to
// read. It is wide enough for a real question (a sentence or two in any
// language) and narrow enough that it stays one line in a supervisor's report.
const ElicitationQuestionMaxRunes = 200

// BoundElicitationQuestion returns the bounded, single-line form of the question
// a run is blocked on, and whether it had to be cut.
//
// Whitespace is collapsed because a question can be several paragraphs and the
// places this travels are single-line summaries; a multi-line question would
// otherwise break the line it is rendered on. Cutting is by rune, so a
// multi-byte character is never split into an invalid one, and the caller gets
// the truncation as a fact rather than a suffix it would have to parse: a
// consumer that sees the marker knows the text is incomplete, and one that does
// not see it knows the text is whole. An empty question stays empty and is not
// marked as cut, because there was nothing to cut.
func BoundElicitationQuestion(message string) (string, bool) {
	runes := []rune(strings.Join(strings.Fields(message), " "))
	if len(runes) <= ElicitationQuestionMaxRunes {
		return string(runes), false
	}
	return string(runes[:ElicitationQuestionMaxRunes]), true
}
