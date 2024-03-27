package stream_test

import (
	"io/fs"
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

	tests := []struct {
		Name     string
		Mock     testutils.ItemMock
		CalcHash bool
		WantErr  error
		WantItem stream.Item
	}{
		{
			Name: "Item LXD metadata",
			Mock: testutils.MockItem("lxd.tar.xz").WithContent("test-content"),
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
			Mock:     testutils.MockItem("disk.qcow2").WithContent("VM"),
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
			Mock:     testutils.MockItem("root.squashfs").WithContent("container"),
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
			Mock: testutils.MockItem("test/delta.123123.vcdiff").WithContent("vcdiff"),
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
			Mock: testutils.MockItem("test/delta-123.qcow2.vcdiff").WithContent(""),
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
			test.Mock.Create(t, t.TempDir())

			item, err := stream.GetItem(test.Mock.RootDir(), test.Mock.RelPath(), test.CalcHash)
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

	tests := []struct {
		Name        string
		Mock        testutils.VersionMock
		CalcHashes  bool
		WantErr     error
		WantVersion stream.Version
	}{
		{
			Name: "Version is incomplete: missing rootfs",
			Mock: testutils.MockVersion("20141010_1212").AddItems(
				testutils.MockItem("lxd.tar.xz"),
			),
			WantErr: stream.ErrVersionIncomplete,
		},
		{
			Name: "Version is incomplete: missing metadata",
			Mock: testutils.MockVersion("20241010_1212").AddItems(
				testutils.MockItem("rootfs.squashfs"),
				testutils.MockItem("disk.qcow2"),
			),
			WantErr: stream.ErrVersionIncomplete,
		},
		{
			Name: "Valid version without item hashes",
			Mock: testutils.MockVersion("v10").AddItems(
				testutils.MockItem("lxd.tar.xz"),
				testutils.MockItem("disk.qcow2"),
				testutils.MockItem("rootfs.squashfs"),
			),
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
			Mock: testutils.MockVersion("v10").AddItems(
				testutils.MockItem("lxd.tar.xz"),
				testutils.MockItem("disk.qcow2"),
				testutils.MockItem("rootfs.squashfs"),
			),
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
			Mock: testutils.MockVersion("v10").AddItems(
				testutils.MockItem("lxd.tar.xz"),
				testutils.MockItem("disk.qcow2"),
				testutils.MockItem("rootfs.squashfs"),
				testutils.MockItem("delta.2013_12_31.vcdiff"),
				testutils.MockItem("delta.2024_12_31.qcow2.vcdiff"),
			),
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
			test.Mock.Create(t, t.TempDir())

			version, err := stream.GetVersion(test.Mock.RootDir(), test.Mock.RelPath(), test.CalcHashes)
			if test.WantErr != nil {
				assert.ErrorIs(t, err, test.WantErr)
			} else {
				// Set expected item paths in test.
				for _, item := range test.WantVersion.Items {
					item.Path = filepath.Join(test.Mock.RelPath(), item.Name)
					test.WantVersion.Items[item.Name] = item
				}

				require.NoError(t, err)
				assert.Equal(t, &test.WantVersion, version)
			}
		})
	}
}

func TestGetProduct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name        string
		Mock        testutils.ProductMock
		IgnoreItems bool
		WantErr     error
		WantProduct stream.Product
	}{
		{
			Name:    "Product path is invalid: too long",
			Mock:    testutils.MockProduct("images/ubuntu/noble/amd64/desktop/2024.04.01"),
			WantErr: stream.ErrProductInvalidPath,
		},
		{
			Name:    "Product path is invalid: too short",
			Mock:    testutils.MockProduct("images/ubuntu/noble/amd64"),
			WantErr: stream.ErrProductInvalidPath,
		},
		{
			Name: "Product with invalid config",
			Mock: testutils.MockProduct("images-minimal/d/r/a/v").
				AddProductConfig("invalid::config"),
			WantErr: stream.ErrProductInvalidConfig,
		},
		{
			Name: "Product with valid config",
			Mock: testutils.MockProduct("stream/distro/release/arch/variant").
				AddProductConfig("requirements:\n  secure_boot: true"),
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
			Mock: testutils.MockProduct("images/ubuntu/focal/arm64/cloud"),
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
			Mock: testutils.MockProduct("images/ubuntu/focal/arm64/default"),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/focal/default,ubuntu/focal", // Note 2 aliases.
				Distro:       "ubuntu",
				Release:      "focal",
				Architecture: "arm64",
				Variant:      "default",
				Requirements: map[string]string{},
			},
		},
		{
			Name: "Product with multiple complete versions",
			Mock: testutils.MockProduct("images/ubuntu/focal/amd64/cloud").AddVersions(
				testutils.MockVersion("2024_01_01").AddItems(
					testutils.MockItem("lxd.tar.xz"),
					testutils.MockItem("root.squashfs"),
					testutils.MockItem("disk.qcow2"),
				),
				testutils.MockVersion("2024_01_02").AddItems(
					testutils.MockItem("lxd.tar.xz"),
					testutils.MockItem("disk.qcow2"),
				),
				testutils.MockVersion("2024_01_03").AddItems(
					testutils.MockItem("lxd.tar.xz"),
					testutils.MockItem("root.squashfs"),
				),
			),
			IgnoreItems: true,
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
			Mock: testutils.MockProduct("images/ubuntu/lunar/amd64/cloud").AddVersions(
				testutils.MockVersion("2024_01_01").AddItems( // Complete version.
					testutils.MockItem("lxd.tar.xz"),
					testutils.MockItem("root.squashfs"),
					testutils.MockItem("disk.qcow2"),
				),
				testutils.MockVersion("2024_01_02").AddItems( // Missing metadata file (incorrect name).
					testutils.MockItem("lxd2.tar.xz"),
					testutils.MockItem("root.squashfs"),
				),
				testutils.MockVersion("2024_01_03").AddItems( // Missing metadata file.
					testutils.MockItem("root.squashfs"),
					testutils.MockItem("disk.qcow2"),
				),
				testutils.MockVersion("2024_01_04").AddItems( // Missing rootfs.
					testutils.MockItem("lxd.tar.xz"),
				),
			),
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
			Mock: testutils.MockProduct("images/ubuntu/xenial/arm64/default").AddVersions(
				testutils.MockVersion("2024_01_01").AddItems(
					testutils.MockItem("lxd.tar.xz"),
					testutils.MockItem("container.squashfs"),
					testutils.MockItem("vm.qcow2"),
				),
			),
			WantProduct: stream.Product{
				Aliases:      "ubuntu/xenial/default,ubuntu/xenial",
				Distro:       "ubuntu",
				Release:      "xenial",
				Architecture: "arm64",
				Variant:      "default",
				Requirements: map[string]string{},
				Versions: map[string]stream.Version{
					"2024_01_01": {
						Items: map[string]stream.Item{
							"lxd.tar.xz": {
								Size:  12,
								Name:  "lxd.tar.xz",
								Path:  "images/ubuntu/xenial/arm64/default/2024_01_01/lxd.tar.xz",
								Ftype: "lxd.tar.xz",
							},
							"container.squashfs": {
								Size:  12,
								Name:  "container.squashfs",
								Path:  "images/ubuntu/xenial/arm64/default/2024_01_01/container.squashfs",
								Ftype: "squashfs",
							},
							"vm.qcow2": {
								Size:  12,
								Name:  "vm.qcow2",
								Path:  "images/ubuntu/xenial/arm64/default/2024_01_01/vm.qcow2",
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
			p.Create(t, t.TempDir())

			product, err := stream.GetProduct(p.RootDir(), p.RelPath())
			if test.WantErr != nil {
				assert.ErrorIs(t, err, test.WantErr)
				return
			}

			if test.IgnoreItems {
				// Remove all items from the resulting product.
				for id := range product.Versions {
					product.Versions[id] = stream.Version{}
				}
			}

			require.NoError(t, err)
			assert.Equal(t, &test.WantProduct, product)
		})
	}
}

func TestGetProducts(t *testing.T) {
	t.Parallel()

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
				testutils.MockProduct("images-daily/ubuntu/jammy/amd64/cloud").AddVersions(
					testutils.MockVersion("2024_01_01").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
				),

				// Ensure products with multiple valid version are included.
				testutils.MockProduct("images-daily/ubuntu/jammy/arm64/desktop").AddVersions(
					testutils.MockVersion("2023").WithFiles("lxd.tar.xz", "root.squashfs", "disk.qcow2"),
					testutils.MockVersion("2024").WithFiles("lxd.tar.xz", "root.squashfs"),
					testutils.MockVersion("2025").WithFiles("lxd.tar.xz", "disk.qcow2"),
				),

				// Ensure incomplete versions are ignored.
				testutils.MockProduct("images-daily/alpine/edge/amd64/cloud").AddVersions(
					testutils.MockVersion("v1").WithFiles("lxd.tar.xz"),               // Incomplete
					testutils.MockVersion("v2").WithFiles("disk.qcow2"),               // Incomplete
					testutils.MockVersion("v3").WithFiles("lxd.tar.xz", "disk.qcow2"), // Complete
					testutils.MockVersion("v4"),                                       // Incomplete
				),

				// Ensure products with all incomplete versions are not included.
				testutils.MockProduct("images-daily/alpine/edge/amd64/cloud").AddVersions(
					testutils.MockVersion("01").WithFiles("lxd.tar.xz"),
					testutils.MockVersion("02").WithFiles("disk.qcow2"),
					testutils.MockVersion("03").WithFiles(),
				),

				// Ensure empty products (products with no versions) are not included.
				testutils.MockProduct("images-daily/alpine/3.19/amd64/cloud"),

				// Ensure products on invalid path are not included.
				testutils.MockProduct("images-daily/invalid/product").AddVersions(
					testutils.MockVersion("one").WithFiles("lxd.tar.xz", "disk.qcow2"),
					testutils.MockVersion("two").WithFiles("lxd.tar.xz", "root.squashfs"),
					testutils.MockVersion("three"),
				),
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
						"2023": {},
						"2024": {},
						"2025": {},
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
						"v3": {},
					},
				},
			},
		},
	}

	for _, test := range tests {
		tmpDir := t.TempDir()

		ps := test.Mock

		for _, p := range ps {
			p.Create(t, tmpDir)
		}

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

func TestDoesNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Mock    testutils.Mock
		WantErr error
	}{
		{
			Name:    "Item does not exist",
			Mock:    testutils.MockItem("lxd.tar.xz"),
			WantErr: fs.ErrNotExist,
		},
		{
			Name:    "Version does not exist",
			Mock:    testutils.MockVersion("20230211_1212"),
			WantErr: fs.ErrNotExist,
		},
		{
			Name:    "Product does not exist",
			Mock:    testutils.MockProduct("images/ubuntu/noble/amd64/desktop"),
			WantErr: fs.ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			var err error

			switch test.Mock.(type) {
			case testutils.ItemMock:
				_, err = stream.GetItem(test.Mock.RootDir(), test.Mock.RelPath(), false)
			case testutils.VersionMock:
				_, err = stream.GetVersion(test.Mock.RootDir(), test.Mock.RelPath(), false)
			case testutils.ProductMock:
				_, err = stream.GetProduct(test.Mock.RootDir(), test.Mock.RelPath())
			default:
				require.Fail(t, "Unknown mock type")
			}

			assert.ErrorIs(t, err, test.WantErr)
		})
	}
}
