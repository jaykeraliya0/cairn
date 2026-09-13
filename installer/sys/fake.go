package sys

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// FakeRunner records commands instead of running them. Tests use it to assert
// that a given Config produces exactly the argv an install should.
type FakeRunner struct {
	// Calls holds every command passed to Run, in order.
	Calls []Cmd
	// Stdins holds what each Run call was given on standard input, indexed
	// alongside Calls, so tests can check that secrets went over a pipe.
	Stdins []string
	// OutputCalls holds every command passed to Output, in order.
	OutputCalls []Cmd

	// Outputs maps a command line ("blkid -s UUID -o value /dev/sda2") to what
	// Output should return for it. An unlisted command returns "".
	Outputs map[string]string
	// Errs maps a command line to an error Run or Output should return.
	Errs map[string]error
}

// Run records c and returns any error registered for it.
func (f *FakeRunner) Run(_ context.Context, c Cmd) error {
	stdin := ""
	if c.Stdin != nil {
		b, err := io.ReadAll(c.Stdin)
		if err != nil {
			return fmt.Errorf("fake runner: reading stdin for %s: %w", c.Name, err)
		}
		stdin = string(b)
	}
	f.Calls = append(f.Calls, c)
	f.Stdins = append(f.Stdins, stdin)
	return f.Errs[c.String()]
}

// Output records the command and returns the registered canned output.
func (f *FakeRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	c := Cmd{Name: name, Args: args}
	f.OutputCalls = append(f.OutputCalls, c)
	if err := f.Errs[c.String()]; err != nil {
		return "", err
	}
	return f.Outputs[c.String()], nil
}

// CommandLines returns every recorded Run command as a string, which makes for
// readable assertions and readable failure output.
func (f *FakeRunner) CommandLines() []string {
	lines := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		lines[i] = c.String()
	}
	return lines
}

// Ran reports whether any recorded command line contains every given fragment.
func (f *FakeRunner) Ran(fragments ...string) bool {
	return f.Find(fragments...) != ""
}

// Find returns the first recorded command line containing every fragment, or
// "" if there is none.
func (f *FakeRunner) Find(fragments ...string) string {
	for _, line := range f.CommandLines() {
		match := true
		for _, fragment := range fragments {
			if !strings.Contains(line, fragment) {
				match = false
				break
			}
		}
		if match {
			return line
		}
	}
	return ""
}
