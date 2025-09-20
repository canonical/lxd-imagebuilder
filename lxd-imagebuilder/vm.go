package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	lxdShared "github.com/canonical/lxd/shared"
	"golang.org/x/sys/unix"

	"github.com/canonical/lxd-imagebuilder/shared"
)

type vm struct {
	imageFile  string
	loopDevice string
	rootFS     string
	rootfsDir  string
	bootfsDir  string
	size       uint64
	ctx        context.Context
}

func newVM(ctx context.Context, imageFile, rootfsDir, fs string, size uint64) (*vm, error) {
	if fs == "" {
		fs = "ext4"
	}

	if !slices.Contains([]string{"btrfs", "ext4"}, fs) {
		return nil, fmt.Errorf("Unsupported fs: %s", fs)
	}

	if size == 0 {
		size = 4294967296
	}

	return &vm{ctx: ctx, imageFile: imageFile, rootfsDir: rootfsDir, rootFS: fs, size: size}, nil
}

func (v *vm) getLoopDev() string {
	return v.loopDevice
}

func (v *vm) getRootfsDevFile() string {
	if v.loopDevice == "" {
		return ""
	}

	return fmt.Sprintf("%sp2", v.loopDevice)
}

func (v *vm) getUEFIDevFile() string {
	if v.loopDevice == "" {
		return ""
	}

	return fmt.Sprintf("%sp1", v.loopDevice)
}

func (v *vm) findRootfsDevUUID() (rootUUID string, err error) {
	rootfsDevFile := v.getRootfsDevFile()
	if rootfsDevFile == "" {
		return "", fmt.Errorf("Failed to get rootfs device name")
	}

	var out strings.Builder
	if err = shared.RunCommand(v.ctx, nil, &out, "blkid", "-o", "export", rootfsDevFile); err != nil {
		return "", fmt.Errorf("Failed to get rootfs device UUID: %w", err)
	}

	fields := strings.FieldsSeq(out.String())
	for field := range fields {
		if strings.HasPrefix(field, "UUID=") {
			rootUUID = field
			break
		}
	}

	if rootUUID == "" {
		return "", fmt.Errorf("No rootfs device UUID found")
	}

	return rootUUID, nil
}

func (v *vm) createEmptyDiskImage() error {
	f, err := os.Create(v.imageFile)
	if err != nil {
		return fmt.Errorf("Failed to open %s: %w", v.imageFile, err)
	}

	defer f.Close()

	err = f.Chmod(0600)
	if err != nil {
		return fmt.Errorf("Failed to chmod %s: %w", v.imageFile, err)
	}

	err = f.Truncate(int64(v.size))
	if err != nil {
		return fmt.Errorf("Failed to create sparse file %s: %w", v.imageFile, err)
	}

	return nil
}

func (v *vm) createPartitions(args ...[]string) error {
	if len(args) == 0 {
		args = [][]string{
			{"--zap-all"},
			{"--new=1::+100M", "-t 1:EF00"},
			{"--new=2::", "-t 2:8300"},
		}
	}

	for _, cmd := range args {
		err := shared.RunCommand(v.ctx, nil, nil, "sgdisk", append([]string{v.imageFile}, cmd...)...)
		if err != nil {
			return fmt.Errorf("Failed to create partitions: %w", err)
		}
	}

	return nil
}

func (v *vm) lsblkLoopDevice() (parseMajorMinor func(int) (uint32, uint32, error), num int, err error) {
	var out strings.Builder

	// Ensure the partitions are accessible. This part is usually only needed
	// if building inside of a container.
	err = shared.RunCommand(v.ctx, nil, &out, "lsblk", "--raw", "--output", "MAJ:MIN", "--noheadings", v.loopDevice)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to list block devices: %w", err)
	}

	lsblkOutput := strings.TrimSpace(out.String())
	deviceNumbers := strings.Split(lsblkOutput, "\n")

	// Output sample:
	// 7:1    -- loop device
	// 259:2  -- partition 1
	// 259:3  -- partition 2
	parseMajorMinor = func(i int) (major uint32, minor uint32, err error) {
		if i >= len(deviceNumbers) {
			return 0, 0, fmt.Errorf("Failed to parse major and minor for %d >= %d", i, num)
		}

		fields := strings.Split(deviceNumbers[i], ":")

		majorNum, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("Failed to parse %q: %w", fields[0], err)
		}

		minorNum, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("Failed to parse %q: %w", fields[1], err)
		}

		return uint32(majorNum), uint32(minorNum), nil
	}

	return parseMajorMinor, len(deviceNumbers), nil
}

func (v *vm) losetup() error {
	var out strings.Builder

	err := shared.RunCommand(v.ctx, nil, &out, "losetup", "-P", "-f", "--show", v.imageFile)
	if err != nil {
		return fmt.Errorf("Failed to setup loop device: %w", err)
	}

	err = shared.RunCommand(v.ctx, nil, nil, "udevadm", "settle")
	if err != nil {
		return fmt.Errorf("Failed to wait loop device ready: %w", err)
	}

	v.loopDevice = strings.TrimSpace(out.String())
	return nil
}

func (v *vm) mountImage() error {
	// If loopDevice is set, it probably is already mounted.
	if v.loopDevice != "" {
		return nil
	}

	err := v.losetup()
	if err != nil {
		return err
	}

	parseMajorMinor, num, err := v.lsblkLoopDevice()
	if err != nil {
		return err
	} else if num != 3 {
		return fmt.Errorf("Failed to list block devices")
	}

	if !lxdShared.PathExists(v.getUEFIDevFile()) {
		var major, minor uint32

		major, minor, err = parseMajorMinor(1)
		if err != nil {
			return err
		}

		dev := unix.Mkdev(uint32(major), uint32(minor))

		err = unix.Mknod(v.getUEFIDevFile(), unix.S_IFBLK|0644, int(dev))
		if err != nil {
			return fmt.Errorf("Failed to create block device %q: %w", v.getUEFIDevFile(), err)
		}
	}

	if !lxdShared.PathExists(v.getRootfsDevFile()) {
		var major, minor uint32

		major, minor, err = parseMajorMinor(2)
		if err != nil {
			return err
		}

		dev := unix.Mkdev(uint32(major), uint32(minor))

		err = unix.Mknod(v.getRootfsDevFile(), unix.S_IFBLK|0644, int(dev))
		if err != nil {
			return fmt.Errorf("Failed to create block device %q: %w", v.getRootfsDevFile(), err)
		}
	}

	return nil
}

func (v *vm) umountImage() error {
	// If loopDevice is empty, the image probably isn't mounted.
	if v.loopDevice == "" || !lxdShared.PathExists(v.loopDevice) {
		return nil
	}

	err := shared.RunCommand(v.ctx, nil, nil, "losetup", "-d", v.loopDevice)
	if err != nil {
		return fmt.Errorf("Failed to detach loop device: %w", err)
	}

	// Make sure that p1 and p2 are also removed.
	if lxdShared.PathExists(v.getUEFIDevFile()) {
		err := os.Remove(v.getUEFIDevFile())
		if err != nil {
			return fmt.Errorf("Failed to remove file %q: %w", v.getUEFIDevFile(), err)
		}
	}

	if lxdShared.PathExists(v.getRootfsDevFile()) {
		err := os.Remove(v.getRootfsDevFile())
		if err != nil {
			return fmt.Errorf("Failed to remove file %q: %w", v.getRootfsDevFile(), err)
		}
	}

	v.loopDevice = ""
	return nil
}

func (v *vm) createRootFS() error {
	if v.loopDevice == "" {
		return errors.New("Disk image not mounted")
	}

	switch v.rootFS {
	case "btrfs":
		err := shared.RunCommand(v.ctx, nil, nil, "mkfs.btrfs", "-f", "-L", "rootfs", v.getRootfsDevFile())
		if err != nil {
			return fmt.Errorf("Failed to create btrfs filesystem: %w", err)
		}

		// Create the root subvolume as well

		err = shared.RunCommand(v.ctx, nil, nil, "mount", "-t", v.rootFS, v.getRootfsDevFile(), v.rootfsDir)
		if err != nil {
			return fmt.Errorf("Failed to mount %q at %q: %w", v.getRootfsDevFile(), v.rootfsDir, err)
		}

		defer func() {
			_ = v.umountPartition(v.rootfsDir)
		}()

		return shared.RunCommand(v.ctx, nil, nil, "btrfs", "subvolume", "create", fmt.Sprintf("%s/@", v.rootfsDir))
	case "ext4":
		return shared.RunCommand(v.ctx, nil, nil, "mkfs.ext4", "-F", "-b", "4096", "-i 8192", "-m", "0", "-L", "rootfs", "-E", "resize=536870912", v.getRootfsDevFile())
	}

	return nil
}

func (v *vm) createUEFIFS() error {
	if v.loopDevice == "" {
		return errors.New("Disk image not mounted")
	}

	return shared.RunCommand(v.ctx, nil, nil, "mkfs.vfat", "-F", "32", "-n", "UEFI", v.getUEFIDevFile())
}

func (v *vm) mountRootPartition() error {
	if v.loopDevice == "" {
		return errors.New("Disk image not mounted")
	}

	switch v.rootFS {
	case "btrfs":
		return shared.RunCommand(v.ctx, nil, nil, "mount", v.getRootfsDevFile(), v.rootfsDir, "-t", v.rootFS, "-o", "defaults,discard,nobarrier,commit=300,noatime,subvol=/@")
	case "ext4":
		return shared.RunCommand(v.ctx, nil, nil, "mount", v.getRootfsDevFile(), v.rootfsDir, "-t", v.rootFS, "-o", "discard,nobarrier,commit=300,noatime,data=writeback")
	}

	return nil
}

func (v *vm) mountUEFIPartition() error {
	if v.loopDevice == "" {
		return errors.New("Disk image not mounted")
	}

	v.bootfsDir = filepath.Join(v.rootfsDir, "boot", "efi")

	err := os.MkdirAll(v.bootfsDir, 0755)
	if err != nil {
		return fmt.Errorf("Failed to create directory %q: %w", v.bootfsDir, err)
	}

	return shared.RunCommand(v.ctx, nil, nil, "mount", "-t", "vfat", v.getUEFIDevFile(), v.bootfsDir, "-o", "discard")
}

func (v *vm) umountPartition(mountpoint string) (err error) {
	err = v.checkMountpoint(mountpoint)
	if err != nil {
		return
	}

	return shared.RunCommand(v.ctx, nil, nil, "umount", "-R", mountpoint)
}

func (v *vm) checkMountpoint(mountpoint string) (err error) {
	err = shared.RunCommand(v.ctx, nil, nil, "mountpoint", mountpoint)
	if err != nil {
		err = fmt.Errorf("%s not mounted: %w", mountpoint, err)
	}

	return err
}
