package stream_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/testutils"
)

func TestGetItem(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		Name     string
		Mock     testutils.ItemMock
		CalcHash bool
		WantErr  error
		WantItem stream.Item
	}{
		{
			Name: "Item does not exist",
			Mock: func() testutils.ItemMock {
				mock := testutils.MockItem(t, tmpDir, "nonexistent.txt", "")
				_ = os.RemoveAll(mock.AbsPath())
				return mock
			}(),
			WantErr: fs.ErrNotExist,
		},
		{
			Name: "Item LXD metadata",
			Mock: testutils.MockItem(t, tmpDir, "lxd.tar.xz", "test-content"),
			WantItem: stream.Item{
				Size:   12,
				Name:   "lxd.tar.xz",
				Path:   "lxd.tar.xz",
				Ftype:  "lxd.tar.xz",
				SHA256: "",
			},
		},
		{
			Name:     "Item qcow2 with hash",
			Mock:     testutils.MockItem(t, tmpDir, "disk.qcow2", "VM"),
			CalcHash: true,
			WantItem: stream.Item{
				Size:   2,
				Name:   "disk.qcow2",
				Path:   "disk.qcow2",
				Ftype:  "disk-kvm.img",
				SHA256: "8e5abdd396d535012cb3b24b6c998ab6d8f8118fe5c564c21c624c54964464e6",
			},
		},
		{
			Name:     "Item squashfs with hash",
			Mock:     testutils.MockItem(t, tmpDir, "root.squashfs", "container"),
			CalcHash: true,
			WantItem: stream.Item{
				Size:   9,
				Name:   "root.squashfs",
				Path:   "root.squashfs",
				Ftype:  "squashfs",
				SHA256: "a42d519714d616e9411dbceec4b52808bd6b1ee53e6f6497a281d655357d8b71",
			},
		},
		{
			Name: "Item squashfs vcdiff",
			Mock: testutils.MockItem(t, tmpDir, "test/delta.123123.vcdiff", "vcdiff"),
			WantItem: stream.Item{
				Size:      6,
				Name:      "delta.123123.vcdiff",
				Path:      "test/delta.123123.vcdiff",
				Ftype:     "squashfs.vcdiff",
				DeltaBase: "123123",
				SHA256:    "",
			},
		},
		{
			Name: "Item qcow2 vcdiff",
			Mock: testutils.MockItem(t, tmpDir, "test/delta-123.qcow2.vcdiff", ""),
			WantItem: stream.Item{
				Size:      0,
				Name:      "delta-123.qcow2.vcdiff",
				Path:      "test/delta-123.qcow2.vcdiff",
				Ftype:     "disk-kvm.img.vcdiff",
				DeltaBase: "delta-123",
				SHA256:    "",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			item, err := stream.GetItem(tmpDir, test.Mock.RelPath(), test.CalcHash)
			if test.WantErr != nil {
				assert.ErrorIs(t, err, test.WantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, &test.WantItem, item)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		Name        string
		Mock        testutils.VersionMock
		CalcHashes  bool
		WantErr     error
		WantVersion stream.Version
	}{
		{
			Name: "Version does not exist",
			Mock: func() testutils.VersionMock {
				mock := testutils.MockVersion(t, tmpDir, "non-existent-version", "")
				_ = os.RemoveAll(mock.AbsPath())
				return mock
			}(),
			WantErr: fs.ErrNotExist,
		},
		{
			Name:    "Version is incomplete: missing rootfs",
			Mock:    testutils.MockVersion(t, tmpDir, "20141010_1212", "lxd.tar.xz"),
			WantErr: stream.ErrVersionIncomplete,
		},
		{
			Name:    "Version is incomplete: missing metadata",
			Mock:    testutils.MockVersion(t, tmpDir, "20241010_1212", "disk.qcow2", "rootfs.squashfs"),
			WantErr: stream.ErrVersionIncomplete,
		},
		{
			Name: "Valid version without item hashes",
			Mock: testutils.MockVersion(t, tmpDir, "v10", "lxd.tar.xz", "disk.qcow2", "rootfs.squashfs"),
			WantVersion: stream.Version{
				Items: map[string]stream.Item{
					"lxd.tar.xz": {
						Size:  12,
						Name:  "lxd.tar.xz",
						Ftype: "lxd.tar.xz",
					},
					"disk.qcow2": {
						Size:  12,
						Name:  "disk.qcow2",
						Ftype: "disk-kvm.img",
					},
					"rootfs.squashfs": {
						Size:  12,
						Name:  "rootfs.squashfs",
						Ftype: "squashfs",
					},
				},
			},
		},
		{
			Name:       "Valid version with item hashes: Container and VM",
			CalcHashes: true,
			Mock:       testutils.MockVersion(t, tmpDir, "v20", "lxd.tar.xz", "disk.qcow2", "rootfs.squashfs"),
			WantVersion: stream.Version{
				Items: map[string]stream.Item{
					"lxd.tar.xz": {
						Size:                     12,
						Name:                     "lxd.tar.xz",
						Ftype:                    "lxd.tar.xz",
						SHA256:                   "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
						CombinedSHA256DiskKvmImg: "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
						CombinedSHA256SquashFs:   "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
					},
					"disk.qcow2": {
						Size:   12,
						Name:   "disk.qcow2",
						Ftype:  "disk-kvm.img",
						SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
					},
					"rootfs.squashfs": {
						Size:   12,
						Name:   "rootfs.squashfs",
						Ftype:  "squashfs",
						SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
					},
				},
			},
		},
		{
			Name:       "Valid version with item hashes: Container and VM including delta files",
			CalcHashes: true,
			Mock:       testutils.MockVersion(t, tmpDir, "v30", "lxd.tar.xz", "disk.qcow2", "rootfs.squashfs", "delta.2013_12_31.vcdiff", "delta.2024_12_31.qcow2.vcdiff"),
			WantVersion: stream.Version{
				Items: map[string]stream.Item{
					"lxd.tar.xz": {
						Size:                     12,
						Name:                     "lxd.tar.xz",
						Ftype:                    "lxd.tar.xz",
						SHA256:                   "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
						CombinedSHA256DiskKvmImg: "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
						CombinedSHA256SquashFs:   "d9da2d2151ce5c89dfb8e1c329b286a02bd8464deb38f0f4d858486a27b796bf",
					},
					"disk.qcow2": {
						Size:   12,
						Name:   "disk.qcow2",
						Ftype:  "disk-kvm.img",
						SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
					},
					"rootfs.squashfs": {
						Size:   12,
						Name:   "rootfs.squashfs",
						Ftype:  "squashfs",
						SHA256: "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
					},
					"delta.2013_12_31.vcdiff": {
						Size:      12,
						Name:      "delta.2013_12_31.vcdiff",
						Ftype:     "squashfs.vcdiff",
						SHA256:    "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
						DeltaBase: "2013_12_31",
					},
					"delta.2024_12_31.qcow2.vcdiff": {
						Size:      12,
						Name:      "delta.2024_12_31.qcow2.vcdiff",
						Ftype:     "disk-kvm.img.vcdiff",
						SHA256:    "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e",
						DeltaBase: "2024_12_31",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			version, err := stream.GetVersion(tmpDir, test.Mock.RelPath(), test.CalcHashes)
			if test.WantErr != nil {
				assert.ErrorIs(t, err, test.WantErr)
			} else {
				// Set expected item paths in test.
				for _, item := range test.WantVersion.Items {
					item.Path = filepath.Join(test.Mock.RelPath(), item.Name)
					test.WantVersion.Items[item.Name] = item
				}

				require.NoError(t, err)
				require.Equal(t, &test.WantVersion, version)
			}
		})
	}
}

func TestGetProduct(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		Name        string
		Mock        testutils.ProductMock
		IgnoreItems bool
		WantErr     error
		WantProduct stream.Product
	}{
		{
			Name: "Product does not exist",
			Mock: func() testutils.ProductMock {
				mock := testutils.MockProduct(t, tmpDir, "images/ubuntu/noble/amd64/not-exist")
				_ = os.RemoveAll(mock.AbsPath())
				return mock
			}(),
			WantErr: fs.ErrNotExist,
		},
		{
			Name:    "Product path is invalid: too long",
			Mock:    testutils.MockProduct(t, tmpDir, "images/ubuntu/noble/amd64/desktop/2024.04.01"),
			WantErr: stream.ErrProductInvalidPath,
		},
		{
			Name:    "Product path is invalid: too short",
			Mock:    testutils.MockProduct(t, tmpDir, "images/ubuntu/noble/amd64"),
			WantErr: stream.ErrProductInvalidPath,
		},
		{
			Name: "Product with invalid config",
			Mock: testutils.MockProduct(t, tmpDir, "images-minimal/d/r/a/v").
				SetProductConfig("invalid::config"),
			WantErr: stream.ErrProductInvalidConfig,
		},
		{
			Name: "Product with valid config",
			Mock: testutils.MockProduct(t, tmpDir, "stream/distro/release/arch/variant").
				SetProductConfig("requirements:\n  secure_boot: true"),
			WantProduct: stream.Product{
				Aliases:      "distro/release/variant",
				Distro:       "distro",
				Release:      "release",
				Architecture: "arch",
				Variant:      "variant",
				Requirements: map[string]string{
					"secure_boot": "true",
				},
			},
		},
		{
			Name: "Product with no versions (empty)",
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/focal/arm64/cloud"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/focal/cloud",
				Distro:       "ubuntu",
				Release:      "focal",
				Architecture: "arm64",
				Variant:      "cloud",
				Requirements: map[string]string{},
			},
		},
		{
			Name: "Product with default variant",
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/focal/arm64/default"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/focal/default,ubuntu/focal",
				Distro:       "ubuntu",
				Release:      "focal",
				Architecture: "arm64",
				Variant:      "default",
				Requirements: map[string]string{},
			},
		},
		{
			Name:        "Product with with multiple version",
			IgnoreItems: true,
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/focal/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_03", "lxd.tar.xz", "root.squashfs", "disk.qcow2"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/focal/cloud",
				Distro:       "ubuntu",
				Release:      "focal",
				Architecture: "amd64",
				Variant:      "cloud",
				Requirements: map[string]string{},
				Versions: map[string]stream.Version{
					"2024_01_01": {},
					"2024_01_02": {},
					"2024_01_03": {},
				},
			},
		},
		{
			Name:        "Product with incomplete versions",
			IgnoreItems: true,
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/lunar/amd64/cloud").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
				AddVersion("2024_01_02", "lxd.tar.xz").
				AddVersion("2024_01_03", "root.squashfs", "disk.qcow2"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/lunar/cloud",
				Distro:       "ubuntu",
				Release:      "lunar",
				Architecture: "amd64",
				Variant:      "cloud",
				Requirements: map[string]string{},
				Versions: map[string]stream.Version{
					"2024_01_01": {},
				},
			},
		},
		{
			Name:        "Product with with one complete version to test item paths",
			IgnoreItems: false,
			Mock: testutils.MockProduct(t, tmpDir, "images/ubuntu/xenial/amd64/default").
				AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/xenial/default,ubuntu/xenial",
				Distro:       "ubuntu",
				Release:      "xenial",
				Architecture: "amd64",
				Variant:      "default",
				Requirements: map[string]string{},
				Versions: map[string]stream.Version{
					"2024_01_01": {
						Items: map[string]stream.Item{
							"lxd.tar.xz": {
								Size:  12,
								Name:  "lxd.tar.xz",
								Path:  "images/ubuntu/xenial/amd64/default/2024_01_01/lxd.tar.xz",
								Ftype: "lxd.tar.xz",
							},
							"root.squashfs": {
								Size:  12,
								Name:  "root.squashfs",
								Path:  "images/ubuntu/xenial/amd64/default/2024_01_01/root.squashfs",
								Ftype: "squashfs",
							},
							"disk.qcow2": {
								Size:  12,
								Name:  "disk.qcow2",
								Path:  "images/ubuntu/xenial/amd64/default/2024_01_01/disk.qcow2",
								Ftype: "disk-kvm.img",
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			p := test.Mock

			product, err := stream.GetProduct(tmpDir, p.RelPath())
			if test.WantErr != nil {
				require.ErrorIs(t, err, test.WantErr)
				return
			}

			if test.IgnoreItems {
				// Remove all items from the resulting product.
				for id := range product.Versions {
					product.Versions[id] = stream.Version{}
				}
			}

			require.NoError(t, err)
			require.Equal(t, &test.WantProduct, product)
		})
	}
}

func TestGetProducts(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		Name         string
		Mock         []testutils.ProductMock
		IgnoreItems  bool
		WantErr      error
		WantProducts map[string]stream.Product
	}{
		{
			Name: "Test multiple products",
			Mock: []testutils.ProductMock{
				// Ensure products with single valid version are included.
				testutils.MockProduct(t, tmpDir, "images-daily/ubuntu/jammy/amd64/cloud").
					AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2"),

				// Ensure products with multiple valid version are included.
				testutils.MockProduct(t, tmpDir, "images-daily/ubuntu/jammy/arm64/desktop").
					AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
					AddVersion("2024_01_02", "lxd.tar.xz", "root.squashfs").
					AddVersion("2024_01_03", "lxd.tar.xz", "disk.qcow2"),

				// Ensure incomplete versions are ignored.
				testutils.MockProduct(t, tmpDir, "images-daily/alpine/edge/amd64/cloud").
					AddVersion("2024_01_01", "lxd.tar.xz", "root.squashfs", "disk.qcow2").
					AddVersion("2024_01_02", "lxd.tar.xz"). // Incomplete version
					AddVersion("2024_01_03"),               // Incomplete version

				// Ensure products with all incomplete versions are not included.
				testutils.MockProduct(t, tmpDir, "images-daily/alpine/edge/amd64/cloud").
					AddVersion("2024_01_01", "disk.qcow2"). // Incomplete version
					AddVersion("2024_01_02", "lxd.tar.xz"). // Incomplete version
					AddVersion("2024_01_03"),               // Incomplete version

				// Ensure empty products (products with no versions) are not included.
				testutils.MockProduct(t, tmpDir, "images-daily/alpine/3.19/amd64/cloud"),

				// Ensure invalid product paths are ignored.
				testutils.MockProduct(t, tmpDir, "images-daily/invalid/product").
					AddVersion("2024_01_01", "disk.qcow2"). // Incomplete version
					AddVersion("2024_01_02", "lxd.tar.xz"). // Incomplete version
					AddVersion("2024_01_03"),               // Incomplete version
			},
			WantProducts: map[string]stream.Product{
				"ubuntu:jammy:amd64:cloud": {
					Aliases:      "ubuntu/jammy/cloud",
					Distro:       "ubuntu",
					Release:      "jammy",
					Architecture: "amd64",
					Variant:      "cloud",
					Requirements: map[string]string{},
					Versions: map[string]stream.Version{
						"2024_01_01": {},
					},
				},
				"ubuntu:jammy:arm64:desktop": {
					Aliases:      "ubuntu/jammy/desktop",
					Distro:       "ubuntu",
					Release:      "jammy",
					Architecture: "arm64",
					Variant:      "desktop",
					Requirements: map[string]string{},
					Versions: map[string]stream.Version{
						"2024_01_01": {},
						"2024_01_02": {},
						"2024_01_03": {},
					},
				},
				"alpine:edge:amd64:cloud": {
					Aliases:      "alpine/edge/cloud",
					Distro:       "alpine",
					Release:      "edge",
					Architecture: "amd64",
					Variant:      "cloud",
					Requirements: map[string]string{},
					Versions: map[string]stream.Version{
						"2024_01_01": {},
					},
				},
			},
		},
	}

	for _, test := range tests {
		ps := test.Mock

		if len(ps) == 0 {
			require.Fail(t, "Test must include at least one mocked product!")
		}

		products, err := stream.GetProducts(tmpDir, ps[0].StreamName())
		require.NoError(t, err)

		// Ensure expected products are found.
		require.ElementsMatch(t,
			shared.MapKeys(test.WantProducts),
			shared.MapKeys(products),
			"Expected and actual products do not match")

		// Ensure expected product versions are found for each product.
		for id := range products {
			require.ElementsMatchf(t,
				shared.MapKeys(test.WantProducts[id].Versions),
				shared.MapKeys(products[id].Versions),
				"Versions do not match for product %q", id)
		}
	}
}
