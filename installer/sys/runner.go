// Package sys wraps the parts of the machine the installer has to look at or
// shell out to: external commands, block devices, and the CPU/GPU probe.
package sys

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Cmd describes one external command.
//
// Args is a slice, never a string, and nothing here is ever handed to a shell.
// That is deliberate: hostnames, usernames and passphrases come from the user,
// and an argv has no syntax for them to escape into.
type Cmd struct {
	Name string
	Args []string
	// Stdin feeds the command's standard input. Secrets travel this way —
	// passphrases to cryptsetup, passwords to chpasswd — so they never appear
	// in an argv or an environment block that other local processes can read.
	Stdin io.Reader
	// Env adds KEY=VAL entries on top of the inherited environment.
	Env []string
}

// String renders the command for the install log. Stdin is never shown.
func (c Cmd) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Runner executes external commands. apply talks only to this interface, which
// is what lets its tests assert the exact argv of every command an install
// would run without a disk, a chroot, or root.
type Runner interface {
	// Run executes c, streaming its combined output to the log.
	Run(ctx context.Context, c Cmd) error
	// Output executes a command and returns its standard output.
	Output(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner is the Runner that really forks.
type ExecRunner struct {
	// Log receives the command line, then one call per line of output. It may
	// be nil, in which case output is discarded.
	Log func(string)
}

func (r ExecRunner) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(fmt.Sprintf(format, args...))
	}
}

// Run executes c and streams stdout and stderr, interleaved, to r.Log.
func (r ExecRunner) Run(ctx context.Context, c Cmd) error {
	r.logf("$ %s", c)

	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Stdin = c.Stdin
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}

	// Read on this goroutine and wait afterwards: cmd.Wait closes the pipe, so
	// draining it first is what guarantees no output is lost.
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanner.Split(scanProgressLines)
	for scanner.Scan() {
		if line := strings.TrimRight(StripANSI(scanner.Text()), " \t"); line != "" {
			r.logf("%s", line)
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	return nil
}

// Output runs a command and returns its trimmed standard output. Used for the
// short, parseable commands — blkid, lsblk, findmnt — rather than the long
// ones whose output belongs in the log.
func (r ExecRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	// The command line is logged like any other, so the log shows every command
	// the install ran — blkid and genfstab included, not only the long ones.
	// Its standard output is not: that is data for the caller, and genfstab's
	// would otherwise be printed twice.
	r.logf("$ %s", Cmd{Name: name, Args: args})

	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(StripANSI(stderr.String()))
		for _, line := range strings.Split(msg, "\n") {
			if line = strings.TrimRight(line, " \t"); line != "" {
				r.logf("%s", line)
			}
		}
		if msg != "" {
			return "", fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ansiEscape matches CSI sequences (colour, cursor movement) and OSC sequences
// (window titles), which some tools emit even into a pipe.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

// StripANSI removes terminal escape sequences from command output.
//
// Left in, they are counted as printable width by the log view, which
// misaligns every row after them, and they land in the log file as literal
// garbage.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// scanProgressLines splits on carriage returns as well as newlines, so
// pacman's in-place progress bars arrive as a stream of updates instead of
// accumulating into one enormous final line.
func scanProgressLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		// Consume \r\n as a single break rather than emitting a blank line.
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			return i + 2, data[:i], nil
		}
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
