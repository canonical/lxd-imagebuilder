package stream

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	lxdShared "github.com/canonical/lxd/shared"

	"github.com/canonical/lxd-imagebuilder/shared"
)

var (
	// ErrVersionIncomplete indicates that version is missing some files.
	// For a version to be complete, a metadata and at least one root
	// filesystem (qcow2/squashfs) must be present.
	ErrVersionIncomplete = errors.New("Product version is incomplete")

	// ErrProductInvalidPath indicates that product's path is invalid because
	// either the directory on the given path does not exist, or it's path
	// does not match the expected format.
	ErrProductInvalidPath = errors.New("Invalid product path")

	// ErrProductInvalidConfig indicating product's configuration file is invalid.
	ErrProductInvalidConfig = errors.New("Invalid product config")
)

// ItemType is a type of the file that item holds.
type ItemType string

const (
	// ItemTypeMetadata represents the LXD metadata file.
	ItemTypeMetadata = "lxd.tar.xz"

	// ItemTypeSquashfs represents container's root file system (squashfs).
	ItemTypeSquashfs = "squashfs"

	// ItemTypeSquashfsDelta represents container's root file system delta (VCDiff).
	ItemTypeSquashfsDelta = "squashfs.vcdiff"

	// ItemTypeDiskKVM represents VM's root file system (qcow2).
	ItemTypeDiskKVM = "disk-kvm.img"

	// ItemTypeDiskKVMDelta represents VM's root file system delta (VCDiff).
	ItemTypeDiskKVMDelta = "disk-kvm.img.vcdiff"

	// ItemTypeRootTarXz represents root file system as a tarball.
	ItemTypeRootTarXz = "root.tar.xz"
)

// ItemExt is file extension of the the file that item holds.
type ItemExt string

const (
	// ItemExtMetadata is a file extension of LXD metadata file.
	ItemExtMetadata = ".tar.xz"

	// ItemExtSquashfs is a file extension of container's root file system.
	ItemExtSquashfs = ".squashfs"

	// ItemExtSquashfsDelta is a file extension of container's root file system delta (VCDiff).
	ItemExtSquashfsDelta = ".vcdiff"

	// ItemExtDiskKVM is a file extension of VM's root file system.
	ItemExtDiskKVM = ".qcow2"

	// ItemExtDiskKVMDelta is a file extension of VM's root file system delta (VCDiff).
	ItemExtDiskKVMDelta = ".qcow2.vcdiff"
)

// List of item extensions that will be included in a product version.
var allowedItemExtensions = []string{
	ItemExtMetadata,
	ItemExtSquashfs,
	ItemExtSquashfsDelta,
	ItemExtDiskKVM,
	ItemExtDiskKVMDelta,
}

// List of valid product config names.
var productConfigNames = []string{
	"config.yaml",
}

// Item represents a file within a product version.
type Item struct {
	// Name of the file.
	Name string `json:"-"`

	// Type of the file. A known ItemType is used if possible, otherwise,
	// this field is equal to the file's name.
	Ftype string `json:"ftype"`

	// Path of the file relative to the root directory (the directory where
	// the simplestream content is hosted from).
	Path string `json:"path"`

	// Size of file.
	Size int64 `json:"size"`

	// SHA256 hash of the file.
	SHA256 string `json:"sha256,omitempty"`

	// CombinedSHA256DiskKvmImg stores the combined SHA256 hash of the metadata
	// and VM file system (qcow2) files. This field is set only for the metadata
	// item when both files exist in the same product version.
	CombinedSHA256DiskKvmImg string `json:"combined_disk-kvm-img_sha256,omitempty"`

	// CombinedSHA256DiskKvmImg stores the combined SHA256 hash of the metadata
	// and container file system (squashfs) files. This field is set only for
	// the metadata item when both files exist in the same product version.
	CombinedSHA256SquashFs string `json:"combined_squashfs_sha256,omitempty"`

	// CombinedSHA256RootXz stores the combined SHA256 hash of the metadata and
	// root file system tarball files. This field is set only for the metadata
	// item when both files exist in the same product version.
	CombinedSHA256RootXz string `json:"combined_rootxz_sha256,omitempty"`

	// DeltaBase indicates the version from which the delta (.vcdiff) file was
	// calculated from. This field is set only for the delta items.
	DeltaBase string `json:"delta_base,omitempty"`
}

// Version represents a list of items available for the given image version.
type Version struct {
	// Map of items found within the version, where the map key
	// represents file name.
	Items map[string]Item `json:"items,omitempty"`
}

// Product represents a single image with all its available versions.
type Product struct {
	// List of aliases using which the product (image) can be referenced.
	Aliases string `json:"aliases"`

	// Architecture the image was built for. For example amd64.
	Architecture string `json:"arch"`

	// Name of the image distribution.
	Distro string `json:"os"`

	// Name of the image release.
	Release string `json:"release"`

	// Release title or in other words pretty display name.
	ReleaseTitle string `json:"release_title"`

	// Name of the image variant.
	Variant string `json:"variant"`

	// Map of image versions, where the map key represents the version name.
	Versions map[string]Version `json:"versions,omitempty"`

	// Map of the requirements that need to be satisfied in order for the
	// image to work. Map key represents the configuration key and map
	// value the expected configuration value.
	Requirements map[string]string `json:"requirements"`
}

// ID returns the ID of the product.
func (p Product) ID() string {
	return fmt.Sprintf("%s:%s:%s:%s", p.Distro, p.Release, p.Architecture, p.Variant)
}

// RelPath returns the product's path relative to the stream's root directory.
func (p Product) RelPath() string {
	return filepath.Join(p.Distro, p.Release, p.Architecture, p.Variant)
}

// ProductConfig contains additional data for all product versions (if found).
type ProductConfig struct {
	// Map of the image requirements.
	Requirements map[string]string `json:"requirements"`
}

// ProductCatalog contains all products.
type ProductCatalog struct {
	// ContentID (e.g. images).
	ContentID string `json:"content_id"`

	// Format of the product catalog (e.g. products:1.0).
	Format string `json:"format"`

	// Data type of the product catalog (e.g. image-downloads).
	DataType string `json:"datatype"`

	// Map of products, where the map key represents a product ID.
	Products map[string]Product `json:"products"`
}

// GetProductCatalog generates a catalog of products located on the given path.
func GetProductCatalog(rootDir string, streamName string) (*ProductCatalog, error) {
	products, err := GetProducts(rootDir, streamName, true)
	if err != nil {
		return nil, err
	}

	imageStream := ProductCatalog{
		ContentID: "images",
		DataType:  "image-downloads",
		Format:    "products:1.0",
		Products:  products,
	}

	return &imageStream, nil
}

// GetProducts traverses through the directories on the given path and retrieves
// a map of found products.
func GetProducts(rootDir string, streamRelPath string, calcHashes bool) (map[string]Product, error) {
	streamPath := filepath.Join(rootDir, streamRelPath)

	products := make(map[string]Product)

	// Traverse recursively through directories and populate map of products.
	err := filepath.WalkDir(streamPath, func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get product path relative to rootDir.
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		// Get product on the given path.
		product, err := GetProduct(rootDir, relPath, calcHashes)
		if err != nil {
			if errors.Is(err, ErrProductInvalidPath) {
				// Ignore invalid product paths.
				return nil
			}

			return err
		}

		// Skip products with no versions (empty products).
		if len(product.Versions) == 0 {
			return nil
		}

		products[product.ID()] = *product
		return nil
	})
	if err != nil {
		return nil, err
	}

	return products, nil
}

// GetProduct reads the product on the given path including all of its versions.
// Product's relative path must match the predetermined format, otherwise, an error
// is returned.
func GetProduct(rootDir string, productRelPath string, calcHashes bool) (*Product, error) {
	productPath := filepath.Join(rootDir, productRelPath)
	productPathFormat := "stream/distribution/release/architecture/variant"
	productPathLength := len(strings.Split(productPathFormat, string(os.PathSeparator)))

	// Ensure product relative path matches the required format.
	parts := strings.Split(productRelPath, string(os.PathSeparator))
	if len(parts) < productPathLength || len(parts) > productPathLength {
		return nil, fmt.Errorf("%w: path %q does not match the required format %q",
			ErrProductInvalidPath, productRelPath, productPathFormat)
	}

	// Ensure product path is a directory.
	info, err := os.Stat(productPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrProductInvalidPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: not a directory", ErrProductInvalidPath)
	}

	// New product.
	p := Product{
		Variant:      parts[len(parts)-1],
		Architecture: parts[len(parts)-2],
		Release:      parts[len(parts)-3],
		Distro:       parts[len(parts)-4],
		Requirements: make(map[string]string, 0),
	}

	// Evaluate aliases.
	aliases := []string{fmt.Sprintf("%s/%s/%s", p.Distro, p.Release, p.Variant)}
	if p.Variant == "default" {
		aliases = append(aliases, fmt.Sprintf("%s/%s", p.Distro, p.Release))
	}

	p.Aliases = strings.Join(aliases, ",")

	// Check product content.
	files, err := os.ReadDir(productPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read product contents: %w", err)
	}

	for _, f := range files {
		if f.IsDir() {
			versionRelPath := filepath.Join(productRelPath, f.Name())

			// Parse product version.
			version, err := GetVersion(rootDir, versionRelPath, calcHashes)
			if err != nil {
				if errors.Is(err, ErrVersionIncomplete) {
					// Ignore incomplete versions.
					continue
				}

				return nil, err
			}

			if p.Versions == nil {
				p.Versions = make(map[string]Version)
			}

			p.Versions[f.Name()] = *version
		} else if lxdShared.ValueInSlice(f.Name(), productConfigNames) {
			configPath := filepath.Join(productPath, f.Name())

			// Parse product config.
			config, err := shared.ReadYAMLFile(configPath, &ProductConfig{})
			if err != nil {
				return nil, fmt.Errorf("product %q: %w: %w", productRelPath, ErrProductInvalidConfig, err)
			}

			// Apply config to product.
			p.Requirements = config.Requirements
		}
	}

	return &p, nil
}

// GetVersion retrieves metadata for a single version, by reading directory
// files and converting those that should be incuded in the product catalog
// into items.
func GetVersion(rootDir string, versionRelPath string, calcHashes bool) (*Version, error) {
	versionPath := filepath.Join(rootDir, versionRelPath)

	version := Version{
		Items: make(map[string]Item),
	}

	// Get files on version path.
	files, err := os.ReadDir(versionPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() || !shared.HasSuffix(file.Name(), allowedItemExtensions...) {
			// Skip directories and disallowed items.
			continue
		}

		itemPath := filepath.Join(versionRelPath, file.Name())
		item, err := GetItem(rootDir, itemPath, calcHashes)
		if err != nil {
			return nil, err
		}

		version.Items[file.Name()] = *item
	}

	// Ensure version has at metadata and at least one rootfs (container, vm).
	isVersionComplete := false

	// Calculate combined hashes.
	metaItem, ok := version.Items[ItemTypeMetadata]
	if ok {
		metaItemPath := filepath.Join(versionPath, metaItem.Name)

		for _, i := range version.Items {
			itemHash := ""
			itemPath := filepath.Join(versionPath, i.Name)

			if calcHashes {
				itemHash, err = shared.FileHash(sha256.New(), metaItemPath, itemPath)
				if err != nil {
					return nil, err
				}
			}

			switch i.Ftype {
			case ItemTypeDiskKVM:
				metaItem.CombinedSHA256DiskKvmImg = itemHash
				isVersionComplete = true

			case ItemTypeSquashfs:
				metaItem.CombinedSHA256SquashFs = itemHash
				isVersionComplete = true

			case ItemTypeRootTarXz:
				metaItem.CombinedSHA256RootXz = itemHash
			}
		}

		version.Items[ItemTypeMetadata] = metaItem
	}

	// At least metadata and one of squashfs or qcow2 files must exist
	// for the version to be considered valid.
	if !isVersionComplete {
		return nil, fmt.Errorf("%w: %q", ErrVersionIncomplete, versionRelPath)
	}

	return &version, nil
}

// GetItem retrieves item metadata for the file on a given path.
func GetItem(rootDir string, itemRelPath string, calcHash bool) (*Item, error) {
	itemPath := filepath.Join(rootDir, itemRelPath)

	file, err := os.Stat(itemPath)
	if err != nil {
		return nil, err
	}

	item := Item{}
	item.Name = file.Name()
	item.Size = file.Size()
	item.Path = itemRelPath

	if calcHash {
		hash, err := shared.FileHash(sha256.New(), itemPath)
		if err != nil {
			return nil, err
		}

		item.SHA256 = hash
	}

	switch filepath.Ext(itemPath) {
	case ItemExtSquashfs:
		item.Ftype = ItemTypeSquashfs

	case ItemExtDiskKVM:
		item.Ftype = ItemTypeDiskKVM

	case ".vcdiff":
		parts := strings.Split(item.Name, ".")
		if strings.HasSuffix(item.Name, ItemExtDiskKVMDelta) {
			item.Ftype = ItemTypeDiskKVMDelta
			item.DeltaBase = parts[len(parts)-3]
		} else {
			item.Ftype = ItemTypeSquashfsDelta
			item.DeltaBase = parts[len(parts)-2]
		}

	default:
		item.Ftype = item.Name
	}

	return &item, nil
}
