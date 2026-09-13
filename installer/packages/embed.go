// Package packages carries the package lists Cairn is built from.
//
// build.sh reads the same files from the source tree: target-packages.x86_64
// becomes the system image, nvidia-packages.x86_64 the Nvidia side repository.
// They are embedded here so the installer and its tests can check themselves
// against exactly the lists the ISO was built from.
package packages

import (
	_ "embed"
	"strings"
)

//go:embed target-packages.x86_64
var targetList string

//go:embed nvidia-packages.x86_64
var nvidiaList string

// Target returns the packages the system image is built from.
func Target() []string { return parse(targetList) }

// Nvidia returns every package the Nvidia side repository is built from.
func Nvidia() []string { return parse(nvidiaList) }

// parse strips '#' comments and blank lines, mirroring the sed expression in
// build.sh's read_package_list.
func parse(list string) []string {
	var out []string
	for _, line := range strings.Split(list, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
