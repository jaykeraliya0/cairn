package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"
)

// Disk is one whole block device the installer could install onto.
type Disk struct {
	// Path is the device node, e.g. "/dev/nvme0n1".
	Path string
	// Size is the device size in bytes.
	Size int64
	// Model is the vendor's model string, often empty on virtual disks.
	Model string
	// Transport is how the disk is attached ("nvme", "sata", "usb", ...).
	Transport string
}

// Label renders a disk the way the picker shows it.
func (d Disk) Label() string {
	parts := []string{d.Path, HumanBytes(d.Size)}
	if d.Model != "" {
		parts = append(parts, d.Model)
	}
	if d.Transport != "" {
		parts = append(parts, "("+d.Transport+")")
	}
	return strings.Join(parts, "  ")
}

// lsblkOutput mirrors the shape of `lsblk -J`.
type lsblkOutput struct {
	Blockdevices []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Size   int64  `json:"size"`
		Model  string `json:"model"`
		Type   string `json:"type"`
		RO     bool   `json:"ro"`
		Tran   string `json:"tran"`
		RM     bool   `json:"rm"`
		Vendor string `json:"vendor"`
	} `json:"blockdevices"`
}

// ListDisks returns the whole disks that are safe to offer as install targets:
// real disks only, nothing read-only, and never the device the live ISO is
// running from.
func ListDisks(ctx context.Context, r Runner) ([]Disk, error) {
	out, err := r.Output(ctx, "lsblk", "--json", "--bytes", "--nodeps",
		"--output", "NAME,PATH,SIZE,MODEL,TYPE,RO,TRAN,RM,VENDOR")
	if err != nil {
		return nil, fmt.Errorf("listing block devices: %w", err)
	}

	var parsed lsblkOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil, fmt.Errorf("parsing lsblk output: %w", err)
	}

	liveDisk := LiveBootDisk(ctx, r)

	var disks []Disk
	for _, d := range parsed.Blockdevices {
		switch {
		case d.Type != "disk", d.RO:
			continue
		// Loop, ram and device-mapper nodes are never install targets, and on
		// the live ISO the squashfs itself shows up as a loop device.
		case strings.HasPrefix(d.Name, "loop"),
			strings.HasPrefix(d.Name, "ram"),
			strings.HasPrefix(d.Name, "zram"),
			strings.HasPrefix(d.Name, "dm-"),
			strings.HasPrefix(d.Name, "sr"):
			continue
		// Installing over the disk currently being booted from would pull the
		// filesystem out from under the running installer.
		case liveDisk != "" && d.Path == liveDisk:
			continue
		}

		model := strings.TrimSpace(d.Model)
		if model == "" {
			model = strings.TrimSpace(d.Vendor)
		}
		disks = append(disks, Disk{
			Path:      d.Path,
			Size:      d.Size,
			Model:     model,
			Transport: strings.TrimSpace(d.Tran),
		})
	}
	return disks, nil
}

// LiveBootDisk returns the whole disk the live ISO booted from, or "" when
// that cannot be determined (booting from PXE or a virtual CD, say). Mirrors
// the findmnt/lsblk lookup the original shell installer did.
func LiveBootDisk(ctx context.Context, r Runner) string {
	source, err := r.Output(ctx, "findmnt", "-no", "SOURCE", "/run/archiso/bootmnt")
	if err != nil || source == "" {
		return ""
	}
	parent, err := r.Output(ctx, "lsblk", "-no", "PKNAME", source)
	if err != nil {
		return ""
	}
	// A partition on a disk gives the disk; a whole device gives nothing.
	if parent = strings.TrimSpace(parent); parent == "" {
		return ""
	}
	return "/dev/" + strings.Fields(parent)[0]
}

// PartitionName returns the device node for partition n of a disk.
//
// Devices whose name already ends in a digit — nvme0n1, mmcblk0, loop0 — take
// a "p" separator so the partition number stays readable; everything else gets
// the number appended directly.
func PartitionName(disk string, n int) string {
	base := path.Base(disk)
	if base != "" && unicode.IsDigit(rune(base[len(base)-1])) {
		return fmt.Sprintf("%sp%d", disk, n)
	}
	return fmt.Sprintf("%s%d", disk, n)
}

// HumanBytes renders a byte count with a binary unit.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}
