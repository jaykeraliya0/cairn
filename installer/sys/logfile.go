package sys

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// LogFile is the install log on disk.
//
// The progress screen only ever holds the last few thousand lines, and it is
// gone the moment the user quits to a shell — which is exactly when they need
// the error. This keeps every line, in order, where `cat` can reach it.
type LogFile struct {
	mu sync.Mutex
	f  *os.File
}

// OpenLogFile creates or truncates the log at path. Truncating rather than
// appending means the file only ever describes the most recent attempt, so a
// retry after a failure does not leave the old error sitting above the new
// one.
func OpenLogFile(path string) (*LogFile, error) {
	// 0600: the log holds no secrets — passwords travel on stdin and are never
	// echoed — but it does hold the disk layout and the username, and it has
	// no reason to be readable by anyone but root.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening the install log: %w", err)
	}

	l := &LogFile{f: f}
	l.Write("cairn-install log, started " + time.Now().Format(time.RFC3339))
	return l, nil
}

// Write appends one line. Safe for concurrent use, and a nil LogFile is a
// no-op, so callers do not have to guard against the log failing to open.
func (l *LogFile) Write(line string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.f, line)
}

// Close flushes and closes the file.
func (l *LogFile) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
