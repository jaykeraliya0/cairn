package sys

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecRunnerLogsTheCommandAndBothStreams(t *testing.T) {
	var lines []string
	r := ExecRunner{Log: func(s string) { lines = append(lines, s) }}

	err := r.Run(context.Background(), Cmd{
		Name: "sh",
		Args: []string{"-c", "echo to-stdout; echo to-stderr >&2; exit 3"},
	})
	if err == nil {
		t.Fatal("a command exiting 3 returned nil")
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{"$ sh -c", "to-stdout", "to-stderr"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the log is missing %q:\n%s", want, joined)
		}
	}
}

func TestExecRunnerOutputLogsTheCommandAndItsErrors(t *testing.T) {
	var lines []string
	r := ExecRunner{Log: func(s string) { lines = append(lines, s) }}

	out, err := r.Output(context.Background(), "sh", "-c", "echo data")
	if err != nil || out != "data" {
		t.Fatalf("Output = %q, %v", out, err)
	}
	// The command is logged; its standard output is data for the caller and
	// must not be printed into the log too.
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "$ sh -c") {
		t.Errorf("log = %q, want only the command line", lines)
	}

	lines = nil
	if _, err := r.Output(context.Background(), "sh", "-c", "echo broke >&2; exit 1"); err == nil {
		t.Fatal("a failing command returned nil")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "broke") {
		t.Errorf("the failing command's stderr never reached the log: %q", lines)
	}
}

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;32m==>\x1b[0m Installing": "==> Installing",
		"\x1b]0;window title\x07text":     "text",
		"plain line":                      "plain line",
		"\x1b[2K\x1b[1Gprogress 42%":      "progress 42%",
	}
	for in, want := range cases {
		if got := StripANSI(in); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogFileKeepsEveryLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.log")

	l, err := OpenLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Write("$ pacstrap -K /mnt base")
	l.Write("error: failed to commit transaction")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"started", "$ pacstrap -K /mnt base", "error: failed to commit transaction"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the log file is missing %q:\n%s", want, body)
		}
	}

	// A retry truncates, so the old error is not left sitting above the new run.
	l2, err := OpenLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = l2.Close()
	body, _ = os.ReadFile(path)
	if strings.Contains(string(body), "failed to commit") {
		t.Error("reopening the log kept the previous attempt's lines")
	}

	// A nil log is a no-op, so a log that failed to open cannot crash the install.
	var none *LogFile
	none.Write("ignored")
	if err := none.Close(); err != nil {
		t.Errorf("closing a nil log = %v", err)
	}
}
