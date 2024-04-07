package stream

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/canonical/lxd-imagebuilder/shared"
)

var (
	// ErrVersionIncomplete indicates that version is missing some files.
	// For a version to be complete, a metadata and at least one root
	// filesystem (qcow2/squashfs) must be present.
	ErrVersionIncomplete = errors.New("Product version is incomplete")

	// ErrVersionInvalidImageConfig indicates version's image config is invalid.
	ErrVersionInvalidImageConfig = errors.New("Product version has invalid image config")

	// ErrProductInvalidPath indicates that product's path is invalid because
	// either the directory on the given path does not exist, or it's path
	// does not match the expected format.
	ErrProductInvalidPath = errors.New("Invalid product path")
)

// Static list of file names.
const (
	// FileChecksumSHA256 is the name of the checksum file containing SHA256 hashes.
	FileChecksumSHA256 = "SHA256SUMS"

	// FileImageConfig is the name of the file that contains additional information
	// about the version.
	FileImageConfig = "image.yaml"
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

// Item represents a file within a product version.
type Item struct {
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
	// incomplete version is either a hidden directory which is considered
	// partially uploaded version, or does not contain both the metadata
	// and at least one rootfs file (squashfs or qcow2).
	incomplete bool `json:"-"`

	// Checksums of files within the version.
	Checksums map[string]string `json:"-"`

	// ImageConfig contains additional information about the product version.
	ImageConfig shared.DefinitionSimplestream `json:"-"`

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
	Distro string `json:"distro"`

	// Pretty name of the operating system.
	OS string `json:"os"`

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

// NewCatalog creates a new product catalog.
func NewCatalog(streamName string, products map[string]Product) *ProductCatalog {
	if products == nil {
		products = make(map[string]Product)
	}

	return &ProductCatalog{
		ContentID: streamName,
		DataType:  "image-downloads",
		Format:    "products:1.0",
		Products:  products,
	}
}

// Option to modify the fetching behavior.
type Option func(*options)

type options struct {
	includeIncomplete bool
	calcHashes        bool
}

func newOptions(opts ...Option) *options {
	o := &options{}

	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	return o
}

// WithIncompleteVersions ensures incomplete versions are included when
// retrieving a version or products.
func WithIncompleteVersions(val bool) Option {
	return func(o *options) {
		o.includeIncomplete = val
	}
}

// WithHashes ensures that item hashes are calculated.
func WithHashes(val bool) Option {
	return func(o *options) {
		o.calcHashes = val
	}
}

// GetProducts traverses through the directories on the given path and retrieves
// a map of found products.
func GetProducts(rootDir string, streamRelPath string, options ...Option) (map[string]Product, error) {
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
		product, err := GetProduct(rootDir, relPath, options...)
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
func GetProduct(rootDir string, productRelPath string, options ...Option) (*Product, error) {
	productPath := filepath.Join(rootDir, productRelPath)
	productPathFormat := "stream/distribution/release/architecture/variant"
	productPathLength := len(strings.Split(productPathFormat, string(os.PathSeparator)))

	// Ensure product relative path matches the required format.
	parts := strings.Split(productRelPath, string(os.PathSeparator))
	if len(parts) < productPathLength || len(parts) > productPathLength {
		return nil, fmt.Errorf("%w: path %q does not match the required format %q", ErrProductInvalidPath, productRelPath, productPathFormat)
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

	// Check product content.
	files, err := os.ReadDir(productPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read product contents: %w", err)
	}

	var aliases []string
	var osName string

	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		versionRelPath := filepath.Join(productRelPath, f.Name())

		// Parse product version.
		version, err := GetVersion(rootDir, versionRelPath, options...)
		if err != nil {
			if errors.Is(err, ErrVersionIncomplete) {
				// Ignore incomplete versions.
				continue
			}

			return nil, err
		}

		// Apply image config if version is complete.
		if !version.incomplete {
			// Reset old values.
			aliases = []string{}
			p.Requirements = make(map[string]string)

			// Set pretty OS name.
			osName = version.ImageConfig.DistroName

			// Set product requirements.
			for _, req := range version.ImageConfig.Requirements {
				// Apply requirements if filter matches the current product.
				// Note that instance types are not supported because requirements
				// are applied to the product itself and not a specific version.
				if shared.ApplyFilter(&req.DefinitionFilter, p.Release, p.Architecture, p.Variant, "", 0) {
					for k, v := range req.Requirements {
						p.Requirements[k] = v
					}
				}
			}

			// Evaluate additional aliases.
			for release, releaseAliases := range version.ImageConfig.ReleaseAliases {
				if release != p.Release {
					// Skip aliases for other releases.
					continue
				}

				for _, releaseAlias := range strings.Split(releaseAliases, ",") {
					aliases = append(aliases, CreateAliases(p.Distro, releaseAlias, p.Variant)...)
				}
			}
		}

		if p.Versions == nil {
			p.Versions = make(map[string]Version)
		}

		p.Versions[f.Name()] = *version
	}

	// Prepend default aliases.
	aliases = append(CreateAliases(p.Distro, p.Release, p.Variant), aliases...)
	p.Aliases = strings.Join(aliases, ",")

	// Set OS name.
	if osName != "" {
		p.OS = osName
	} else {
		p.OS = cases.Title(language.English).String(p.Distro)
	}

	return &p, nil
}

// GetVersion retrieves metadata for a single version, by reading directory
// files and converting those that should be incuded in the product catalog
// into items. For the relevant items, the file hashes are calculated, if
// calcHashes is set to true.
func GetVersion(rootDir string, versionRelPath string, options ...Option) (*Version, error) {
	opts := newOptions(options...)
	versionPath := filepath.Join(rootDir, versionRelPath)

	// Hidden versions are considered incomplete, as they may contain
	// partially uploaded files.
	if strings.HasPrefix(filepath.Base(versionPath), ".") && !opts.includeIncomplete {
		return nil, fmt.Errorf("%w (hidden version): %q", ErrVersionIncomplete, versionRelPath)
	}

	version := Version{
		Items:      make(map[string]Item),
		incomplete: true,
	}

	// Get files on version path.
	files, err := os.ReadDir(versionPath)
	if err != nil {
		return nil, err
	}

	// Extract relevant items from the version directory.
	for _, file := range files {
		if file.IsDir() {
			// Skip directories.
			continue
		}

		if shared.HasSuffix(file.Name(), allowedItemExtensions...) {
			// Get an item and calculate its hash if necessary.
			itemRelPath := filepath.Join(versionRelPath, file.Name())
			item, err := GetItem(rootDir, itemRelPath, options...)
			if err != nil {
				return nil, err
			}

			version.Items[file.Name()] = *item
		} else if file.Name() == FileChecksumSHA256 {
			// Read the checksum file and convert it to a map
			// of filename and checksum pairs.
			checksumPath := filepath.Join(versionPath, file.Name())
			version.Checksums, err = ReadChecksumFile(checksumPath)
			if err != nil {
				return nil, fmt.Errorf("Failed to read checksums file: %w", err)
			}
		} else if file.Name() == FileImageConfig {
			// Read the image config file.
			configPath := filepath.Join(versionPath, file.Name())
			config, err := shared.ReadYAMLFile(configPath, &shared.Definition{})
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrVersionInvalidImageConfig, err)
			}

			version.ImageConfig = config.Simplestream
		}
	}

	// Check whether version is complete, and calculate combined hashes if necessary.
	metaItem, ok := version.Items[ItemTypeMetadata]
	if ok {
		metaItemPath := filepath.Join(versionPath, ItemTypeMetadata)

		for itemName, item := range version.Items {
			if !slices.Contains([]string{ItemTypeSquashfs, ItemTypeDiskKVM, ItemTypeRootTarXz}, item.Ftype) {
				// Skip files that are not required for combined checksum.
				continue
			}

			itemHash := ""

			if opts.calcHashes {
				// Calculate combined hash for the item.
				itemPath := filepath.Join(versionPath, itemName)
				itemHash, err = shared.FileHash(sha256.New(), metaItemPath, itemPath)
				if err != nil {
					return nil, err
				}
			}

			switch item.Ftype {
			case ItemTypeDiskKVM:
				metaItem.CombinedSHA256DiskKvmImg = itemHash
				version.incomplete = false

			case ItemTypeSquashfs:
				metaItem.CombinedSHA256SquashFs = itemHash
				version.incomplete = false

			case ItemTypeRootTarXz:
				metaItem.CombinedSHA256RootXz = itemHash
			}
		}

		version.Items[ItemTypeMetadata] = metaItem
	}

	// At least metadata and one of squashfs or qcow2 files must exist
	// for the version to be considered complete.
	if version.incomplete && !opts.includeIncomplete {
		return nil, fmt.Errorf("%w: %q", ErrVersionIncomplete, versionRelPath)
	}

	return &version, nil
}

// GetItem retrieves item metadata for the file on a given path. If calcHash is
// set to true, the file's hash is calculated.
func GetItem(rootDir string, itemRelPath string, options ...Option) (*Item, error) {
	opts := newOptions(options...)
	itemPath := filepath.Join(rootDir, itemRelPath)

	file, err := os.Stat(itemPath)
	if err != nil {
		return nil, err
	}

	item := Item{}
	item.Size = file.Size()
	item.Path = itemRelPath

	if opts.calcHashes {
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
		parts := strings.Split(file.Name(), ".")
		if strings.HasSuffix(file.Name(), ItemExtDiskKVMDelta) {
			item.Ftype = ItemTypeDiskKVMDelta
			item.DeltaBase = parts[len(parts)-3]
		} else {
			item.Ftype = ItemTypeSquashfsDelta
			item.DeltaBase = parts[len(parts)-2]
		}

	default:
		item.Ftype = file.Name()
	}

	return &item, nil
}

// ReadChecksumFile reads a checksum file and returns a map of filename
// checksum pairs.
func ReadChecksumFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	checksums := make(map[string]string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Trim all leading and trailing whitespace.
		line := strings.TrimSpace(scanner.Text())

		// Split the line into checksum and filename.
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		checksum := parts[0]
		filename := strings.TrimSpace(parts[1])

		checksums[filename] = checksum
	}

	return checksums, nil
}

// CreateAliases creates aliases from the given distro, release, and variant.
// It appends them to the aliases slice and returns the updated slice.
func CreateAliases(distro string, release string, variant string) []string {
	// Use path.Join for aliases to ignore OS specific filepath separator.
	aliases := []string{path.Join(distro, release, variant)}

	// If release is "current" create an additional alias without release.
	if release == "current" {
		aliases = append(aliases, path.Join(distro, variant))
	}

	// If variant is "default" create an additional alias without variant.
	if variant == "default" {
		if release == "current" {
			// If release is also "current", remove release and variant.
			aliases = append(aliases, distro)
		} else {
			aliases = append(aliases, path.Join(distro, release))
		}
	}

	return aliases
}
