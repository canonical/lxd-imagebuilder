package shared

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	lxdShared "github.com/canonical/lxd/shared"
	"golang.org/x/sys/unix"
)

// Unpack unpacks a tarball.
func Unpack(file string, path string) error {
	extractArgs, extension, _, err := lxdShared.DetectCompression(file)
	if err != nil {
		return err
	}

	command := ""
	args := []string{}
	var reader io.Reader
	if strings.HasPrefix(extension, ".tar") {
		command = "tar"
		args = append(args, "--restrict", "--force-local")
		args = append(args, "-C", path, "--numeric-owner", "--xattrs-include=*")
		args = append(args, extractArgs...)
		args = append(args, "-")

		f, err := os.Open(file)
		if err != nil {
			return err
		}

		defer f.Close()

		reader = f
	} else if strings.HasPrefix(extension, ".squashfs") {
		// unsquashfs does not support reading from stdin,
		// so ProgressTracker is not possible.
		command = "unsquashfs"
		args = append(args, "-f", "-d", path, "-n", file)
	} else {
		return fmt.Errorf("Unsupported image format: %s", extension)
	}

	err = lxdShared.RunCommandWithFds(context.TODO(), reader, nil, command, args...)
	if err != nil {
		// We can't create char/block devices in unpriv containers so ignore related errors.
		if command == "unsquashfs" {
			var runError *lxdShared.RunError

			ok := errors.As(err, &runError)
			if !ok || runError.StdErr().String() == "" {
				return err
			}

			// Confirm that all errors are related to character or block devices.
			found := false
			for line := range strings.SplitSeq(runError.StdErr().String(), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				if !strings.Contains(line, "failed to create block device") {
					continue
				}

				if !strings.Contains(line, "failed to create character device") {
					continue
				}

				// We found an actual error.
				found = true
			}

			if !found {
				// All good, assume everything unpacked.
				return nil
			}
		}

		// Check if we ran out of space
		fs := unix.Statfs_t{}

		err1 := unix.Statfs(path, &fs)
		if err1 != nil {
			return err1
		}

		// Check if we're running out of space
		if int64(fs.Bfree) < 10 {
			return fmt.Errorf("Unable to unpack image, run out of disk space")
		}

		return fmt.Errorf("Unpack failed: %w", err)
	}

	return nil
}
