package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/canonical/lxd-imagebuilder/shared"
)

func lsblkOutputHelper(t *testing.T, v *vm, args [][]string) (rb func()) {
	t.Helper()
	// Prepare image file
	f, err := os.CreateTemp("", "lsblkOutput*.raw")
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	v.imageFile = f.Name()
	v.size = 2e8
	defer func() {
		if err != nil {
			os.RemoveAll(v.imageFile)
		}
	}()

	err = v.createEmptyDiskImage()
	if err != nil {
		t.Fatal(err)
	}

	v.ctx = context.Background()
	// Format disk
	if args == nil {
		err = v.createPartitions()
	} else {
		err = v.createPartitions(args...)
	}

	if err != nil {
		t.Fatal(err)
	}

	// losetup
	err = v.losetup()
	if err != nil {
		t.Fatal(err)
	}

	rb = func() {
		err := shared.RunCommand(v.ctx, nil, nil, "losetup", "-d", v.loopDevice)
		if err != nil {
			t.Fatal(err)
		}

		err = os.RemoveAll(v.imageFile)
		if err != nil {
			t.Fatal(err)
		}
	}

	return
}

func TestLsblkOutput(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip()
	}

	tcs := []struct {
		name string
		args [][]string
		want int
	}{
		{"DiskEmpty", [][]string{{"--zap-all"}}, 1},
		{"UEFI", nil, 3},
		{"MBR", [][]string{{"--new=1::"}, {"--gpttombr"}}, 2},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			v := &vm{}
			rb := lsblkOutputHelper(t, v, tc.args)
			defer rb()
			parse, num, err := v.lsblkLoopDevice()
			if err != nil || num != tc.want {
				t.Fatal(err, num, tc.want)
			}

			for i := range num {
				major, minor, err := parse(i)
				if err != nil {
					t.Fatal(err)
				}

				if major == 0 && minor == 0 {
					t.Fatal(major, minor)
				}
			}
		})
	}
}

func TestNewVMSizeParsing(t *testing.T) {
	tcs := []struct {
		name      string
		size      string
		wantBytes uint64
		wantErr   bool
	}{
		{
			name:      "GiB",
			size:      "2GiB",
			wantBytes: 2 * 1024 * 1024 * 1024,
		},
		{
			name:      "GB",
			size:      "1GB",
			wantBytes: 1 * 1000 * 1000 * 1000,
		},
		{
			name:      "MiB",
			size:      "512MiB",
			wantBytes: 512 * 1024 * 1024,
		},
		{
			name:      "MB",
			size:      "100MB",
			wantBytes: 100 * 1000 * 1000,
		},
		{
			name:      "KiB",
			size:      "1024KiB",
			wantBytes: 1024 * 1024,
		},
		{
			name:      "bytes",
			size:      "4096B",
			wantBytes: 4096,
		},
		{
			name:      "zero uses default",
			size:      "0B",
			wantBytes: 4294967296,
		},
		{
			name:    "negative is invalid",
			size:    "-1",
			wantErr: true,
		},
		{
			name:    "invalid",
			size:    "notasize",
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			v, err := newVM(context.Background(), "", t.TempDir(), "ext4", tc.size)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantBytes, v.size)
		})
	}
}
