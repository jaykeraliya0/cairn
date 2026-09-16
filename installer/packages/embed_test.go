package packages

import (
	"strings"
	"testing"
)

func TestTarget(t *testing.T) {
	pkgs := Target()
	if len(pkgs) < 20 {
		t.Fatalf("the target list parsed to only %d packages: %v", len(pkgs), pkgs)
	}

	// Everything every machine needs has to be in the image, because nothing
	// else is installed on the target except Nvidia's drivers.
	for _, want := range []string{
		"base", "linux", "linux-firmware", "mkinitcpio", "btrfs-progs", "hyprland", "sddm",
		"intel-ucode", "amd-ucode", "cryptsetup", "mesa", "vulkan-intel", "vulkan-radeon",
		// The wifi and bluetooth managers the default waybar clicks and
		// Hyprland keybinds open. A bind whose program is not in the image is
		// the quiet kind of broken: the window simply never appears.
		"impala", "bluetui",
	} {
		if !contains(pkgs, want) {
			t.Errorf("the target list is missing %q", want)
		}
	}

	assertClean(t, "target", pkgs)
}

func TestNvidia(t *testing.T) {
	pkgs := Nvidia()
	for _, want := range []string{"nvidia-open", "nvidia-open-dkms", "nvidia-utils", "egl-wayland", "linux-headers"} {
		if !contains(pkgs, want) {
			t.Errorf("the nvidia list is missing %q", want)
		}
	}
	assertClean(t, "nvidia", pkgs)
}

func TestNvidiaStaysOutOfTheImage(t *testing.T) {
	// The whole point of the side repository is that machines without an
	// Nvidia card never carry 2.4 GiB of its drivers. One stray line in the
	// target list would put them in every install.
	target := Target()
	for _, pkg := range Nvidia() {
		if contains(target, pkg) {
			t.Errorf("%q is in both lists, so it ships inside the image", pkg)
		}
	}
}

// assertClean checks that comment and blank-line stripping actually worked; a
// stray "#" reaching pacstrap fails the build minutes in.
func assertClean(t *testing.T, list string, pkgs []string) {
	t.Helper()

	seen := map[string]bool{}
	for _, pkg := range pkgs {
		switch {
		case pkg == "":
			t.Errorf("the %s list yielded an empty entry", list)
		case strings.ContainsAny(pkg, "# \t"):
			t.Errorf("the %s list yielded %q, which is not a package name", list, pkg)
		case seen[pkg]:
			t.Errorf("the %s list contains %q twice", list, pkg)
		}
		seen[pkg] = true
	}
}

func TestParseStripsCommentsAndBlanks(t *testing.T) {
	got := parse("# leading comment\n\n  base  \nlinux # trailing comment\n\t\n  # indented comment\nsudo\n")
	want := []string{"base", "linux", "sudo"}

	if len(got) != len(want) {
		t.Fatalf("parse = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parse = %v, want %v", got, want)
		}
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
