package testutils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

const (
	// ItemDefaultContent is the default content of the item.
	ItemDefaultContent = "test-content"

	// ItemDefaultContentSHA is the SHA256 hash of the default item content.
	ItemDefaultContentSHA = "0a3666a0710c08aa6d0de92ce72beeb5b93124cce1bf3701c9d6cdeb543cb73e"
)

// Mock is an interface for all mock types.
type Mock interface {
	RootDir() string
	RelPath() string
	AbsPath() string
}

// common implements Mock interface.
type common struct {
	rootDir string
	relPath string
}

func (c common) RootDir() string {
	return c.rootDir
}

func (c common) RelPath() string {
	return c.relPath
}

func (c common) AbsPath() string {
	return filepath.Join(c.rootDir, c.relPath)
}

func (c *common) setRootDir(t *testing.T, rootDir string) {
	// Validation to prevent common issues during development.
	require.NotEmpty(t, rootDir, "Attempt to set an empty root dir for a mock!")
	if c.rootDir != "" && c.rootDir != rootDir {
		require.FailNow(t, c.rootDir, "Attempt to change a root dir for a mock!")
	}

	c.rootDir = rootDir
}

// ProductMock is a mock for a product directory structure.
type ProductMock struct {
	common

	// Versions of the product.
	versions []VersionMock

	// When creating a product, the catalog is built after
	// version indicated by catalogAfterVersion is created.
	catalogAfterVersion string

	// When creating a product, files age will be modified
	// once a version indicated by setAgeAfterVersion is
	// created.
	setAge             time.Duration
	setAgeAfterVersion string
}

// MockProduct initializes new product mock.
func MockProduct(productRelPath string) ProductMock {
	return ProductMock{
		common: common{
			relPath: productRelPath,
		},
	}
}

// AddVersions adds mocked versions to the product.
func (p ProductMock) AddVersions(versions ...VersionMock) ProductMock {
	p.versions = append(p.versions, versions...)
	return p
}

// AddProductCatalog creates product catalog from the current directory structure.
// It sets a checkpoint for the current state of the product. When the product is
// being created, catalog will be built when the product reaches that state.
func (p ProductMock) AddProductCatalog() ProductMock {
	version := "."
	if len(p.versions) > 0 {
		version = p.versions[len(p.versions)-1].RelPath()
	}

	p.catalogAfterVersion = version
	return p
}

// SetFilesAge modifies age (modification time) of the product files It sets a
// checkpoint for the current state of the product. When the product is being
// created, files age will be modified once the product reaches  that state.
func (p ProductMock) SetFilesAge(age time.Duration) ProductMock {
	version := "."
	if len(p.versions) > 0 {
		version = p.versions[len(p.versions)-1].RelPath()
	}

	p.setAgeAfterVersion = version
	p.setAge = age
	return p
}

// Create creates the mocked product directory structure in the given directory.
// According to the mock's configuration, product catalog and config are created.
func (p *ProductMock) Create(t *testing.T, rootDir string) ProductMock {
	p.setRootDir(t, rootDir)

	// Ensure product dir exists.
	err := os.MkdirAll(p.AbsPath(), os.ModePerm)
	require.NoError(t, err)

	// Do actions after specific version is created.
	runAfterVersion := func(version string) {
		if version == p.catalogAfterVersion {
			mockProductCatalog(t, p.RootDir(), p.StreamName())
		}

		if version == p.setAgeAfterVersion {
			setFilesAge(t, p.RootDir(), p.setAge)
		}
	}

	// Do before any version is created.
	runAfterVersion(".")

	// Create versions.
	for _, v := range p.versions {
		v.Create(t, p.AbsPath())
		runAfterVersion(v.RelPath())
	}

	return *p
}

// StreamName returns the name of the product's stream.
func (p ProductMock) StreamName() string {
	return strings.SplitN(p.relPath, "/", 2)[0]
}

// VersionMock is a mock for a product version directory structure.
type VersionMock struct {
	common

	// Items of the version.
	items []ItemMock

	// Version checksums file content.
	checksums string

	// Image config.
	imageConfig string
}

// MockVersion initializes new product version mock.
func MockVersion(versionRelPath string) VersionMock {
	return VersionMock{
		common: common{
			relPath: versionRelPath,
		},
	}
}

// WithFiles mocks default versions items with the given names. This is used
// as a shorthand for creating multiple items with the same content.
func (v VersionMock) WithFiles(names ...string) VersionMock {
	for _, name := range names {
		v.items = append(v.items, MockItem(name))
	}

	return v
}

// AddItems adds mocked items to the version.
func (v VersionMock) AddItems(items ...ItemMock) VersionMock {
	v.items = append(v.items, items...)
	return v
}

// SetChecksums stores the checksum entries that are written to a file when
// version is created.
func (v VersionMock) SetChecksums(entries ...string) VersionMock {
	v.checksums = strings.Join(entries, "\n") + "\n"
	return v
}

// SetImageConfig sets image config with the given content that is written
// when a product version is created.
func (v VersionMock) SetImageConfig(lines ...string) VersionMock {
	v.imageConfig = strings.Join(lines, "\n")
	return v
}

// Create creates the mocked version directory structure in the given directory.
func (v *VersionMock) Create(t *testing.T, rootDir string) VersionMock {
	v.setRootDir(t, rootDir)

	// Ensure version dir exists.
	err := os.MkdirAll(v.AbsPath(), os.ModePerm)
	require.NoError(t, err, "Failed to create item's directory")

	// Create version items.
	for _, item := range v.items {
		item.Create(t, v.AbsPath())
	}

	// Create checsums file.
	if v.checksums != "" {
		checksumPath := filepath.Join(v.AbsPath(), stream.FileChecksumSHA256)
		err = os.WriteFile(checksumPath, []byte(v.checksums), os.ModePerm)
		require.NoError(t, err)
	}

	// Write image config.
	if v.imageConfig != "" {
		configPath := filepath.Join(v.AbsPath(), stream.FileImageConfig)
		err = os.WriteFile(configPath, []byte(v.imageConfig), os.ModePerm)
		require.NoError(t, err)
	}

	return *v
}

// ItemMock is a mock for a product version item (file) structure.
type ItemMock struct {
	common

	// Item content.
	content string
}

// MockItem initializes new product version item mock. By default,
// the item content is set to ItemDefaultContent.
func MockItem(name string) ItemMock {
	return ItemMock{
		common: common{
			relPath: name,
		},
		content: ItemDefaultContent,
	}
}

// WithContent sets the content of the item.
func (i ItemMock) WithContent(lines ...string) ItemMock {
	i.content = strings.Join(lines, "\n")
	return i
}

// Create creates a mocked file in the given root directory.
func (i *ItemMock) Create(t *testing.T, rootDir string) ItemMock {
	i.setRootDir(t, rootDir)

	// Ensure parent dir exists.
	err := os.MkdirAll(filepath.Dir(i.AbsPath()), os.ModePerm)
	require.NoError(t, err, "Failed to create item's directory")

	// Write item content.
	err = os.WriteFile(i.AbsPath(), []byte(i.content), os.ModePerm)
	require.NoError(t, err, "Failed to write file")

	return *i
}

// mockProductCatalog creates product catalog from the current directory
// structure. It does not generate any delta files and does not include hashes.
func mockProductCatalog(t *testing.T, rootDir string, streamName string) {
	metaDir := filepath.Join(rootDir, "streams", "v1")

	// Get products from the current directory structure.
	products, err := stream.GetProducts(rootDir, streamName)
	require.NoError(t, err)

	// Create product catalog.
	catalog := stream.NewCatalog(products)
	catalogPath := filepath.Join(metaDir, fmt.Sprintf("%s.json", streamName))

	// Ensure catalog's directory exists.
	err = os.MkdirAll(metaDir, os.ModePerm)
	require.NoError(t, err)

	// Write catalog to a file.
	err = shared.WriteJSONFile(catalogPath, catalog)
	require.NoError(t, err)
}

// setFilesAge recursively sets the age (modification time) of the files in the
// given path. This is especially useful for testing removal of dangling files.
func setFilesAge(t *testing.T, path string, age time.Duration) {
	newModTime := time.Now().Add(-age)

	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Chtimes(path, newModTime, newModTime)
	})

	require.NoError(t, err)
}
