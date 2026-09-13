package sys

import (
	"context"
	"testing"
)

func TestPartitionName(t *testing.T) {
	cases := []struct {
		disk     string
		efi      string
		root     string
		platform string
	}{
		{"/dev/sda", "/dev/sda1", "/dev/sda2", "SATA"},
		{"/dev/vda", "/dev/vda1", "/dev/vda2", "virtio"},
		{"/dev/nvme0n1", "/dev/nvme0n1p1", "/dev/nvme0n1p2", "NVMe"},
		{"/dev/mmcblk0", "/dev/mmcblk0p1", "/dev/mmcblk0p2", "eMMC/SD"},
		{"/dev/loop0", "/dev/loop0p1", "/dev/loop0p2", "loop"},
	}
	for _, c := range cases {
		if got := PartitionName(c.disk, 1); got != c.efi {
			t.Errorf("%s: PartitionName(%q, 1) = %q, want %q", c.platform, c.disk, got, c.efi)
		}
		if got := PartitionName(c.disk, 2); got != c.root {
			t.Errorf("%s: PartitionName(%q, 2) = %q, want %q", c.platform, c.disk, got, c.root)
		}
	}
}

// lsblkJSON is a trimmed but structurally faithful `lsblk --json` reply: one
// real NVMe disk, the USB stick the ISO booted from, a squashfs loop device,
// a read-only optical drive, and a partition that must not be offered as a
// whole-disk target.
const lsblkJSON = `{
  "blockdevices": [
    {"name":"loop0","path":"/dev/loop0","size":812343296,"model":null,"type":"loop","ro":true,"tran":null,"rm":false,"vendor":null},
    {"name":"sr0","path":"/dev/sr0","size":1073741824,"model":"QEMU DVD-ROM","type":"rom","ro":true,"tran":"sata","rm":true,"vendor":"QEMU"},
    {"name":"nvme0n1","path":"/dev/nvme0n1","size":512110190592,"model":"Samsung SSD 990","type":"disk","ro":false,"tran":"nvme","rm":false,"vendor":null},
    {"name":"sda","path":"/dev/sda","size":15728640000,"model":"Cruzer Blade","type":"disk","ro":false,"tran":"usb","rm":true,"vendor":"SanDisk"},
    {"name":"sda1","path":"/dev/sda1","size":15728640000,"model":null,"type":"part","ro":false,"tran":null,"rm":true,"vendor":null},
    {"name":"sdb","path":"/dev/sdb","size":2000398934016,"model":null,"type":"disk","ro":false,"tran":"sata","rm":false,"vendor":"WDC     "}
  ]
}`

func newDiskRunner(bootSource, bootParent string) *FakeRunner {
	outputs := map[string]string{
		"lsblk --json --bytes --nodeps --output NAME,PATH,SIZE,MODEL,TYPE,RO,TRAN,RM,VENDOR": lsblkJSON,
	}
	if bootSource != "" {
		outputs["findmnt -no SOURCE /run/archiso/bootmnt"] = bootSource
		outputs["lsblk -no PKNAME "+bootSource] = bootParent
	}
	return &FakeRunner{Outputs: outputs}
}

func TestListDisksExcludesTheLiveMedium(t *testing.T) {
	disks, err := ListDisks(context.Background(), newDiskRunner("/dev/sda1", "sda"))
	if err != nil {
		t.Fatalf("ListDisks: %v", err)
	}

	var paths []string
	for _, d := range disks {
		paths = append(paths, d.Path)
	}
	want := []string{"/dev/nvme0n1", "/dev/sdb"}
	if len(paths) != len(want) {
		t.Fatalf("ListDisks returned %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("ListDisks returned %v, want %v", paths, want)
		}
	}
}

func TestListDisksWithoutALiveMedium(t *testing.T) {
	// Booting from something findmnt cannot resolve must not drop every disk.
	disks, err := ListDisks(context.Background(), newDiskRunner("", ""))
	if err != nil {
		t.Fatalf("ListDisks: %v", err)
	}
	if len(disks) != 3 {
		t.Fatalf("ListDisks returned %d disks, want 3 (nvme0n1, sda, sdb)", len(disks))
	}
}

func TestDiskLabel(t *testing.T) {
	d := Disk{Path: "/dev/nvme0n1", Size: 512110190592, Model: "Samsung SSD 990", Transport: "nvme"}
	want := "/dev/nvme0n1  476.9 GiB  Samsung SSD 990  (nvme)"
	if got := d.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:           "512 B",
		1024:          "1.0 KiB",
		1536:          "1.5 KiB",
		1073741824:    "1.0 GiB",
		512110190592:  "476.9 GiB",
		2000398934016: "1.8 TiB",
	}
	for input, want := range cases {
		if got := HumanBytes(input); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", input, got, want)
		}
	}
}
