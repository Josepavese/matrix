package main

import "testing"

func TestRunSubmitRejectsBlankChannelBeforeContactingRuntime(t *testing.T) {
	previous := runSubmitChannel
	runSubmitChannel = "  "
	t.Cleanup(func() { runSubmitChannel = previous })
	if err := runRunSubmit(runSubmitCmd); err == nil {
		t.Fatal("blank channel must be refused before reading runtime configuration")
	}
}
