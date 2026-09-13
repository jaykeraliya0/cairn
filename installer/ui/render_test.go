package ui

// Render harness. These tests print whole screens instead of asserting on
// them, so the UI can be looked at without booting a VM — the only other way
// to see this code is on the ISO.
//
//	CAIRN_RENDER=1 go test ./ui/ -run TestRender -v
//
// They are skipped by default: the point is the output, not a pass or fail.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaykeraliya0/cairn/installer/locale"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

func renderSize() (int, int) {
	if os.Getenv("CAIRN_NARROW") != "" {
		return 80, 30
	}
	return 110, 38
}

func shot(label, view string, width int) {
	fmt.Printf("\n%s %s\n", strings.Repeat("─", 4), label)
	fmt.Printf("┌%s┐\n", strings.Repeat("─", width))
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		fmt.Printf("│%s│\n", pad(line, width))
	}
	fmt.Printf("└%s┘\n", strings.Repeat("─", width))
}

// realChoices builds Choices from this machine's actual catalogues, because a
// two-item fixture cannot show what a 553-entry picker looks like.
func realChoices(t *testing.T) Choices {
	t.Helper()

	ch := Choices{
		Disks: []sys.Disk{
			{Path: "/dev/nvme0n1", Size: 512110190592, Model: "Samsung SSD 990 PRO", Transport: "nvme"},
			{Path: "/dev/sdb", Size: 2000398934016, Model: "WDC WD20EZBX", Transport: "sata"},
		},
		GPUs:            []sys.GPUVendor{sys.GPUNvidia, sys.GPUAMD},
		Microcode:       "amd-ucode",
		DefaultTimezone: "Asia/Kolkata",
		DefaultLocale:   "en_US.UTF-8",
		DefaultKeymap:   "us",
	}
	if zones, err := locale.Timezones(locale.ZoneinfoDir); err == nil {
		ch.Timezones = zones
	}
	if locales, err := locale.Locales(locale.LocaleGenTpl); err == nil {
		ch.Locales = locale.Ordered(locales)
	}
	if keymaps, err := locale.Keymaps(locale.KeymapsDir); err == nil {
		ch.Layouts = locale.Layouts(keymaps)
	}
	return ch
}

func newRenderWizard(t *testing.T) (*wizard, int) {
	t.Helper()
	w, h := renderSize()
	m := newWizard(realChoices(t))
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m, w
}

func TestRenderWizard(t *testing.T) {
	if os.Getenv("CAIRN_RENDER") == "" {
		t.Skip("set CAIRN_RENDER=1")
	}

	m, width := newRenderWizard(t)
	shot("step 1 — keyboard", m.View(), width)

	typeInto(m, "germ")
	shot("keyboard, searched 'germ'", m.View(), width)

	keys(m, "enter") // keyboard -> language
	keys(m, "enter") // language -> timezone
	typeInto(m, "kolk")
	shot("timezone, searched 'kolk'", m.View(), width)

	keys(m, "enter") // timezone -> disk
	shot("target disk", m.View(), width)

	keys(m, "enter") // disk -> encryption
	keys(m, "y")
	shot("encryption, turned on", m.View(), width)
}

func TestRenderForms(t *testing.T) {
	if os.Getenv("CAIRN_RENDER") == "" {
		t.Skip("set CAIRN_RENDER=1")
	}

	m, width := newRenderWizard(t)
	m.index = 7 // user
	m.open()
	typeInto(m, "jay")
	keys(m, "enter")
	typeInto(m, "hunter2")
	keys(m, "enter")
	typeInto(m, "hunter3")
	keys(m, "enter")
	shot("user account, passwords disagree", m.View(), width)

	m2, _ := newRenderWizard(t)
	m2.index = 5 // layout
	m2.open()
	shot("disk layout", m2.View(), width)

	m3, _ := newRenderWizard(t)
	m3.a.username = "jay"
	m3.a.password = "hunter2"
	m3.a.encrypt = true
	m3.a.passphrase = "open-sesame"
	m3.a.passphrase2 = "open-sesame"
	m3.index = len(m3.steps) - 1
	m3.open()
	shot("review", m3.View(), width)
}

func TestRenderInstall(t *testing.T) {
	if os.Getenv("CAIRN_RENDER") == "" {
		t.Skip("set CAIRN_RENDER=1")
	}

	w, h := renderSize()
	steps := []string{
		"Partitioning /dev/nvme0n1", "Setting up disk encryption", "Creating filesystems",
		"Creating Btrfs subvolumes", "Mounting target", "Installing packages (this takes a while)",
		"Generating fstab", "Creating swapfile", "Installing default configuration",
		"Configuring the new system", "Installing the bootloader", "Unmounting",
	}
	im := newInstallModel(steps, func() {}, make(chan struct{}))
	im.Update(tea.WindowSizeMsg{Width: w, Height: h})
	im.Update(stepMsg{index: 5, total: len(steps)})
	for _, line := range []string{
		"$ pacstrap -K /mnt base linux linux-firmware mkinitcpio btrfs-progs",
		":: Synchronizing package databases...",
		"installing base (3-2)...",
		"installing linux (6.18.1.arch1-1)...",
		"installing hyprland (0.56.1-1)...",
	} {
		im.Update(logMsg(line))
	}
	shot("installing", im.View(), w)

	im.Update(doneMsg{})
	shot("complete", im.View(), w)
}
