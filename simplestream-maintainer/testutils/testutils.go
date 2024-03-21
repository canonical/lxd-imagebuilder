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

type Mock interface {
	RootDir() string
	RelPath() string
	AbsPath() string
}

// common satisfies Mock interface.
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

type ProductMock struct {
	common

	t *testing.T
}

// MockProduct creates product directory on the given path and returns mocked intstance.
func MockProduct(t *testing.T, rootDir string, productRelPath string) ProductMock {
	p := ProductMock{
		common: common{
			rootDir: rootDir,
			relPath: productRelPath,
		},
		t: t,
	}

	// Ensure product directory exists.
	err := os.MkdirAll(p.AbsPath(), os.ModePerm)
	require.NoError(t, err)

	return p
}

// AddVersion ensures version with the given files is created within a product.
func (p ProductMock) AddVersion(version string, files ...string) ProductMock {
	versionPath := filepath.Join(p.relPath, version)
	MockVersion(p.t, p.rootDir, versionPath, files...)
	return p
}

// BuildProductCatalog creates product catalog from the current directory structure.
// Catalog is written to a file on path streams/v1 within rootDir directory.
func (p ProductMock) BuildProductCatalog() ProductMock {
	catalog, err := stream.GetProductCatalog(p.rootDir, p.StreamName())
	require.NoError(p.t, err)

	catalogPath := filepath.Join(p.rootDir, "streams", "v1", fmt.Sprintf("%s.json", p.StreamName()))

	err = os.MkdirAll(filepath.Dir(catalogPath), os.ModePerm)
	require.NoError(p.t, err)

	err = shared.WriteJSONFile(catalogPath, catalog)
	require.NoError(p.t, err)

	return p
}

// SetFilesAge recursively sets the age (modification time) of the product files.
// This is especially useful to test removal of dangling files.
func (p ProductMock) SetFilesAge(age time.Duration) ProductMock {
	newModTime := time.Now().Add(-age)

	err := filepath.WalkDir(p.AbsPath(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Chtimes(path, newModTime, newModTime)
	})

	require.NoError(p.t, err)
	return p
}

// SetProductConfig writes product config with the given content.
func (p ProductMock) SetProductConfig(content string) ProductMock {
	MockItem(p.t, p.AbsPath(), "config.yaml", content)
	return p
}

// StreamName returns the name of the product's stream.
func (p ProductMock) StreamName() string {
	return strings.SplitN(p.relPath, "/", 2)[0]
}

type VersionMock struct {
	common
}

// MockVersion creates product version directory on the given path and uses
// the provided list of file names to populate the directory. All created
// files have same content ("test-content"). If there is no error, mocked
// instance is returned.
func MockVersion(t *testing.T, rootDir string, versionRelPath string, itemNames ...string) VersionMock {
	v := VersionMock{
		common{
			rootDir: rootDir,
			relPath: versionRelPath,
		},
	}

	// Create version items.
	for _, name := range itemNames {
		MockItem(t, v.AbsPath(), name, "test-content")
	}

	return v
}

type ItemMock struct {
	common
}

// MockItem creates a file on the given path. File content is created by concatentating
// lines with a new line symbol. If no error occurrs, a mocked item is returned.
func MockItem(t *testing.T, dir string, name string, lines ...string) ItemMock {
	i := ItemMock{
		common: common{
			rootDir: dir,
			relPath: name,
		},
	}

	// Ensure parent dir exists.
	err := os.MkdirAll(filepath.Dir(i.AbsPath()), os.ModePerm)
	require.NoError(t, err, "Failed to create file's directory")

	// Write item content.
	content := strings.Join(lines, "\n")
	err = os.WriteFile(i.AbsPath(), []byte(content), os.ModePerm)
	require.NoError(t, err, "Failed to write file")

	return i
}
