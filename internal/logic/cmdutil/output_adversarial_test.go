package cmdutil

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ----------------------------------------------------------------------------
// Adversarial output suite
// ----------------------------------------------------------------------------

// captureStreams runs fn with stdout and stderr redirected and returns what
// each stream received.
func captureStreams(t *testing.T, fn func()) (string, string) {
	t.Helper()
	originalOut, originalErr := os.Stdout, os.Stderr
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errReader, errWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outWriter, errWriter
	defer func() { os.Stdout, os.Stderr = originalOut, originalErr }()

	fn()

	_ = outWriter.Close()
	_ = errWriter.Close()
	outBytes, _ := io.ReadAll(outReader)
	errBytes, _ := io.ReadAll(errReader)
	_ = outReader.Close()
	_ = errReader.Close()
	return string(outBytes), string(errBytes)
}

// TestPrintJSONGoesToStdout is the regression guard for machine-readable
// output: `matrix ... > out.json` must capture the JSON. cobra's Print helpers
// write to OutOrStderr, so a command without an explicit output drain would
// send the payload to stderr and leave the file empty.
func TestPrintJSONGoesToStdout(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	payload := map[string]interface{}{"sessions": []string{"a", "b"}, "count": 2}

	stdout, stderr := captureStreams(t, func() {
		if err := PrintJSON(cmd, payload); err != nil {
			t.Errorf("PrintJSON failed: %v", err)
		}
	})

	if !strings.Contains(stdout, `"sessions"`) || !strings.Contains(stdout, `"count": 2`) {
		t.Fatalf("JSON did not reach stdout: stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("nothing may be written to stderr on success, got %q", stderr)
	}
}

// TestPrintJSONHonoursAnExplicitOutput keeps the pipeline-friendly contract for
// commands that do set their own writer.
func TestPrintJSONHonoursAnExplicitOutput(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	var buffer strings.Builder
	cmd.SetOut(&buffer)
	if err := PrintJSON(cmd, map[string]string{"ok": "yes"}); err != nil {
		t.Fatalf("PrintJSON failed: %v", err)
	}
	if !strings.Contains(buffer.String(), `"ok": "yes"`) {
		t.Fatalf("explicit output ignored: %q", buffer.String())
	}
}

// TestPrintJSONReportsUnencodablePayloads keeps a bad payload from producing a
// half-written document.
func TestPrintJSONReportsUnencodablePayloads(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	var buffer strings.Builder
	cmd.SetOut(&buffer)
	if err := PrintJSON(cmd, map[string]interface{}{"bad": make(chan int)}); err == nil {
		t.Fatal("an unencodable payload must be reported")
	}
	if buffer.Len() != 0 {
		t.Fatalf("a failed encode must not write partial output: %q", buffer.String())
	}
}
