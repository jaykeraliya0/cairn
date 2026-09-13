package apply

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestChrootScriptIsValidShell parses the embedded script with bash itself.
//
// It lives inside a Go raw string literal, so nothing else checks it: a stray
// backtick ends the literal (the compiler catches that one), but an unbalanced
// quote, a broken `if` or a missing `fi` compiles perfectly and only fails
// halfway through a real install, inside a chroot, on someone's machine.
func TestChrootScriptIsValidShell(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}

	path := filepath.Join(t.TempDir(), "chroot.sh")
	if err := os.WriteFile(path, []byte(chrootScript), 0o600); err != nil {
		t.Fatal(err)
	}

	// -n parses without running anything.
	if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("the chroot script is not valid bash: %v\n%s", err, out)
	}
}

// TestChrootScriptEnablesTheServicesItsPackagesNeed pins the services to the
// packages that ship them. A package installed with its unit left disabled is
// the quietest kind of broken: bluetooth simply never appears.
func TestChrootScriptEnablesTheServicesItsPackagesNeed(t *testing.T) {
	for _, unit := range []string{
		"iwd.service", "NetworkManager.service", "sddm.service",
		"bluetooth.service", "power-profiles-daemon.service",
	} {
		if !strings.Contains(chrootScript, unit) {
			t.Errorf("the chroot step never enables %s", unit)
		}
	}

	// No PAM edit for gnome-keyring: Arch's sddm package already carries the
	// pam_gnome_keyring.so lines. Appending more would at best duplicate them,
	// so the script must leave the file alone.
	if strings.Contains(chrootScript, ">> /etc/pam.d/sddm") {
		t.Error("the chroot step appends to /etc/pam.d/sddm, which sddm already configures")
	}
}

// TestChrootSubstitutesTheQtHomePlaceholder guards the contract between the
// skeleton and the chroot step.
//
// qt5ct and qt6ct store an *absolute* path to their color scheme, so the files
// shipped in /etc/skel carry an @HOME@ placeholder that only the chroot can
// resolve — it is the first point at which the account, and therefore its home
// directory, exists. Ship the placeholder without the substitution and every
// Qt application silently falls back to its default palette.
func TestChrootSubstitutesTheQtHomePlaceholder(t *testing.T) {
	if !strings.Contains(chrootScript, "@HOME@") {
		t.Fatal("the chroot step never substitutes the @HOME@ placeholder")
	}
	for _, ct := range []string{"qt5ct", "qt6ct"} {
		if !strings.Contains(chrootScript, ct) {
			t.Errorf("the substitution does not cover %s", ct)
		}
	}
	// It has to run after the account exists, or there is no home to write to.
	if strings.Index(chrootScript, "useradd") > strings.Index(chrootScript, "@HOME@") {
		t.Error("the @HOME@ substitution runs before useradd creates the home directory")
	}
}

// TestChrootScriptOnlyReadsVariablesItIsGiven checks every variable the script
// expands against the ones it can actually have.
//
// The script runs under `set -u`, so a name nobody sets does not expand to
// empty — it aborts the install, part-way through configuring the new system.
// That is exactly how a leftover CAIRN_TARGET_USERNAME, the old bash
// installer's name for what chrootEnv passes as CAIRN_USERNAME, broke every
// install. `bash -n` cannot see it: the script parses perfectly.
func TestChrootScriptOnlyReadsVariablesItIsGiven(t *testing.T) {
	available := map[string]bool{}

	// Everything chrootEnv hands the chroot.
	for _, kv := range chrootEnv(testConfig()) {
		name, _, _ := strings.Cut(kv, "=")
		available[name] = true
	}

	// Plus whatever the script assigns for itself: loop variables and plain
	// assignments.
	assigned := regexp.MustCompile(`(?m)(?:^|\s)(?:for\s+([A-Za-z_]\w*)\s+in|([A-Za-z_]\w*)=)`)
	for _, m := range assigned.FindAllStringSubmatch(chrootScript, -1) {
		for _, name := range m[1:] {
			if name != "" {
				available[name] = true
			}
		}
	}

	read := regexp.MustCompile(`\$\{?([A-Za-z_]\w*)`)
	seen := map[string]bool{}
	for _, m := range read.FindAllStringSubmatch(chrootScript, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !available[name] {
			t.Errorf("the chroot script reads $%s, which chrootEnv never sets and the script never "+
				"assigns — under set -u that aborts the install", name)
		}
	}

	if len(seen) == 0 {
		t.Fatal("found no variable references at all; the pattern is broken, not the script")
	}
}

// TestChrootGivesEachInstallItsOwnIdentity covers the per-machine state the
// image deliberately ships without. Skip it and every Cairn install shares one
// machine ID and one pacman signing key.
func TestChrootGivesEachInstallItsOwnIdentity(t *testing.T) {
	for _, want := range []string{
		"systemd-machine-id-setup",
		"pacman-key --init",
		"pacman-key --populate archlinux",
	} {
		if !strings.Contains(chrootScript, want) {
			t.Errorf("the chroot step never runs %q", want)
		}
	}
	if strings.Index(chrootScript, "pacman-key --init") > strings.Index(chrootScript, "pacman-key --populate") {
		t.Error("the keyring is populated before it is initialised")
	}
}
