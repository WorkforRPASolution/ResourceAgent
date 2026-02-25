package collector

import "testing"

func TestIsPseudoFS_LinuxPseudoFilesystems(t *testing.T) {
	c := &DiskCollector{}

	pseudoTypes := []string{"sysfs", "proc", "tmpfs", "cgroup", "cgroup2", "overlay"}
	for _, fs := range pseudoTypes {
		if !c.isPseudoFS(fs) {
			t.Errorf("expected %q to be pseudo FS", fs)
		}
	}
}

func TestIsPseudoFS_WindowsCDROM(t *testing.T) {
	c := &DiskCollector{}

	cdromTypes := []string{"cdfs", "CDFS", "udf", "UDF"}
	for _, fs := range cdromTypes {
		if !c.isPseudoFS(fs) {
			t.Errorf("expected %q (CD-ROM) to be filtered out", fs)
		}
	}
}

func TestIsPseudoFS_RealFilesystems(t *testing.T) {
	c := &DiskCollector{}

	realTypes := []string{"ntfs", "NTFS", "ext4", "xfs", "btrfs", "fat32", "exfat"}
	for _, fs := range realTypes {
		if c.isPseudoFS(fs) {
			t.Errorf("expected %q to NOT be pseudo FS", fs)
		}
	}
}

func TestShouldSkipPartition_ZeroTotal(t *testing.T) {
	c := &DiskCollector{}

	// CD-ROM or empty drive: total=0 → skip
	if !c.shouldSkipPartition(0) {
		t.Error("expected partition with total=0 to be skipped")
	}
}

func TestShouldSkipPartition_NormalDisk(t *testing.T) {
	c := &DiskCollector{}

	// Normal disk: total > 0 → don't skip
	if c.shouldSkipPartition(100 * 1024 * 1024 * 1024) {
		t.Error("expected partition with total > 0 to NOT be skipped")
	}
}
