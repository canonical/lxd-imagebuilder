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
	ErrVersionIncomplete    = errors.New("product version is incomplete")
	ErrProductInvalidPath   = errors.New("invalid product path")
	ErrProductInvalidConfig = errors.New("invalid product config")
)

type ItemType string

const (
	ItemType_Unknown        = ""
	ItemType_Squshfs        = "squashfs"
	ItemType_Squshfs_VCDiff = "squashfs.vcdiff"
	ItemType_DiskKVM        = "disk-kvm.img"
	ItemType_DiskKVM_VCDiff = "disk-kvm.img.vcdiff"
)

var allowedItemSuffixes = []string{
	".tar.xz",
	".squashfs",
	".qcow2",
	".vcdiff",
}

var imageConfigNames = []string{
	"config.yaml",
}

// Item represents a file within a product version.
type Item struct {
	Name                     string `json:"-"`
	Ftype                    string `json:"ftype"`
	Path                     string `json:"path"`
	Size                     int64  `json:"size"`
	SHA256                   string `json:"sha256,omitempty"`
	CombinedSHA256           string `json:"combined_sha256,omitempty"`
	CombinedSHA256DiskKvmImg string `json:"combined_disk-kvm-img_sha256,omitempty"`
	CombinedSHA256SquashFs   string `json:"combined_squashfs_sha256,omitempty"`
	CombinedSHA256RootXz     string `json:"combined_rootxz_sha256,omitempty"`
	DeltaBase                string `json:"delta_base,omitempty"`
}

// Version represents a list of items available for the given image version.
type Version struct {
	Items map[string]Item `json:"items,omitempty"`
}

func (v Version) ItemsOfType(fType string) []Item {
	var items []Item
	for _, item := range v.Items {
		if item.Ftype == fType {
			items = append(items, item)
		}
	}

	return items
}

// Product represents a singe image with all its available versions.
type Product struct {
	Aliases      string             `json:"aliases"`
	Architecture string             `json:"arch"`
	Distro       string             `json:"os"`
	Release      string             `json:"release"`
	ReleaseTitle string             `json:"release_title"`
	Variant      string             `json:"variant"`
	Versions     map[string]Version `json:"versions,omitempty"`
	Requirements map[string]string  `json:"requirements"`
}

func (p Product) ID() string {
	return fmt.Sprintf("%s:%s:%s:%s", p.Distro, p.Release, p.Architecture, p.Variant)
}

func (p Product) RelPath() string {
	return filepath.Join(p.Distro, p.Release, p.Architecture, p.Variant)
}

// ProductConfig contains additional data for all product versions (if found).
type ProductConfig struct {
	// Image requirements.
	Requirements map[string]string `json:"requirements"`
}

// ProductCatalog contains all products.
type ProductCatalog struct {
	ContentID string             `json:"content_id"`
	Format    string             `json:"format"`
	DataType  string             `json:"datatype"`
	Products  map[string]Product `json:"products"`
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
		} else if lxdShared.ValueInSlice(f.Name(), imageConfigNames) {
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
		if file.IsDir() || !shared.HasSuffix(file.Name(), allowedItemSuffixes...) {
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
	metaItem, ok := version.Items["lxd.tar.xz"]
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
			case "disk-kvm.img":
				metaItem.CombinedSHA256DiskKvmImg = itemHash
				isVersionComplete = true

			case "squashfs":
				metaItem.CombinedSHA256SquashFs = itemHash
				isVersionComplete = true

			case "root.tar.xz":
				metaItem.CombinedSHA256RootXz = itemHash
			}
		}

		version.Items["lxd.tar.xz"] = metaItem
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
	case ".squashfs":
		item.Ftype = ItemType_Squshfs
	case ".qcow2":
		item.Ftype = ItemType_DiskKVM
	case ".vcdiff":
		parts := strings.Split(item.Name, ".")
		if strings.HasSuffix(item.Name, ".qcow2.vcdiff") {
			item.Ftype = ItemType_DiskKVM_VCDiff
			item.DeltaBase = parts[len(parts)-3]
		} else {
			item.Ftype = ItemType_Squshfs_VCDiff
			item.DeltaBase = parts[len(parts)-2]
		}

	default:
		item.Ftype = item.Name
	}

	return &item, nil
}
