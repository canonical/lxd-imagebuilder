package webpage

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/canonical/lxd/shared/units"

	"github.com/canonical/lxd-imagebuilder/embed"
	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

// WebPageImageFile represents a file related to a specific image.
// Used for file listings in image details.
type WebPageImageFile struct {
	Name      string
	Path      string
	Date      string
	Size      string
	SizeBytes int64
}

// WebPageImageVersion represents a version of an image.
type WebPageImageVersion struct {
	Name                 string
	Path                 string
	BuildDate            string
	IsStale              bool
	FingerprintContainer string
	FingerprintVM        string

	Files []WebPageImageFile
}

// WebPageImage represents webpage table entries.
type WebPageImage struct {
	Distribution string
	Release      string
	Architecture string
	Variant      string
	IsStale      bool
	Aliases      []string
	Requirements map[string]string

	Versions []WebPageImageVersion
}

// WebPage represents the data that will be used to populate the webpage template.
type WebPage struct {
	FaviconURL      string
	LogoURL         string
	Title           string
	Paragraphs      []template.HTML
	FooterCopyright string
	FooterUpdatedAt string

	Images []WebPageImage
}

// NewWebPage creates initializes a webpage struct from the given product catalog.
// If the image paths from the catalog are detected in the provided rootDir, the
// files are inspected and their metadata is included in the image version details.
func NewWebPage(rootDir string, catalog stream.ProductCatalog) (*WebPage, error) {
	// This is hardcoded in case we ever decide to manage index.html
	// using a configuration file. In such case, we just have to parse
	// those values and the rest of the code will work as expected.
	page := WebPage{
		Title:           "LXD Images",
		FaviconURL:      "https://raw.githubusercontent.com/canonical/lxd/main/doc/.sphinx/_static/favicon.ico",
		LogoURL:         "https://raw.githubusercontent.com/canonical/lxd/main/doc/.sphinx/_static/tag.png",
		FooterCopyright: fmt.Sprintf("© %d Canonical Ltd.", time.Now().Year()),
		FooterUpdatedAt: fmt.Sprintf("Last updated: %s UTC", time.Now().UTC().Format("02 Jan 2006 (15:04)")),
		Paragraphs: []template.HTML{
			template.HTML("Images hosted on this server are available in LXD through the predefined remote <code>images:</code>. For detailed instructions about LXD image management, please refer to our <a href='https://documentation.ubuntu.com/lxd/en/latest/howto/images_manage/'>How to Manage Images</a> guide in the official documentation."),
			template.HTML("Images are built daily and we retain the last 2 successful builds of each image for up to 15 days. Thus, if a particular build fails on any given day, the previous successful builds will remain accessible."),
			template.HTML("If you encounter any issues with the images hosted on our server or have suggestions for improvement, please let us know by <a href='https://github.com/canonical/lxd/issues/new'>opening an issue</a> in the LXD repository."),
		},
		Images: []WebPageImage{},
	}

	// Sort productIds by name.
	productIds := shared.MapKeys(catalog.Products)
	slices.Sort(productIds)

	// Iterate over products and their versions to extract hosted images.
	for _, id := range productIds {
		product := catalog.Products[id]
		versionIds := shared.MapKeys(product.Versions)

		if len(versionIds) == 0 {
			// Ignore empty products
			continue
		}

		image := WebPageImage{
			Aliases:      strings.Split(product.Aliases, ","),
			Distribution: product.OS,
			Release:      product.Release,
			Architecture: product.Architecture,
			Variant:      product.Variant,
			Requirements: product.Requirements,
		}

		// Sort version ids in reverse order, so that the first version
		// is the most recent one.
		slices.Sort(versionIds)
		slices.Reverse(versionIds)

		// Iterate over product's image versions and extract relevant
		// inforation and files metadata.
		for _, id := range versionIds {
			versionDir := filepath.Join(catalog.ContentID, product.RelPath())
			version, err := parseVersions(product, rootDir, versionDir, id)
			if err != nil {
				return nil, err
			}

			image.Versions = append(image.Versions, *version)
		}

		page.Images = append(page.Images, image)
	}

	return &page, nil
}

// Write parses the webpage template, populates it, and writes it to index.html
// in the rootDir. File is first written to a temporary file and then moved
// to the final destination to avoid partial writes in case of errors.
func (p WebPage) Write(rootDir string) error {
	path := filepath.Join(rootDir, "index.html")
	pathTmp := filepath.Join(rootDir, ".index.html.tmp")

	t, err := template.ParseFS(embed.GetTemplates(), "templates/index.html")
	if err != nil {
		return err
	}

	defer os.Remove(pathTmp)

	f, err := os.OpenFile(pathTmp, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer f.Close()

	err = t.Execute(f, p)
	if err != nil {
		return err
	}

	return os.Rename(pathTmp, path)
}

// parseVersions extracts image version metadata, and referenced files.
func parseVersions(product stream.Product, rootDir string, versionDir string, versionId string) (*WebPageImageVersion, error) {
	version := WebPageImageVersion{
		Name:      versionId,
		Path:      filepath.Join("/", versionDir, versionId),
		BuildDate: "N/A",
	}

	// Converts timestamp from format "YYYYMMDD_hhmm" into a prettier
	// format "YYYY-MM-DD (hh:mm)".
	timestamp, err := time.Parse("20060102_1504", versionId)
	if err == nil {
		version.BuildDate = formatTime(timestamp)
	}

	// Image is considered stale if older than 8 days.
	if timestamp.Before(time.Now().AddDate(0, 0, -8)) {
		version.IsStale = true
	}

	// Extract files metadata from the version (catalog).
	for _, item := range product.Versions[versionId].Items {
		// Indicate image support for VMs and containers and include
		// respective fingerprint which can be used to launch instance
		// from a particular image version.
		if item.Ftype == stream.ItemTypeMetadata {
			// The first 12 characters of the combined checksum
			// are used as short fingerprint in LXD.
			if len(item.CombinedSHA256SquashFs) > 12 {
				version.FingerprintContainer = item.CombinedSHA256SquashFs[:12]
			}

			if len(item.CombinedSHA256DiskKvmImg) > 12 {
				version.FingerprintVM = item.CombinedSHA256DiskKvmImg[:12]
			}
		}

		version.Files = append(version.Files, WebPageImageFile{
			Name:      filepath.Base(item.Path),
			Path:      item.Path,
			Date:      version.BuildDate,
			Size:      formatSize(item.Size, 2),
			SizeBytes: item.Size,
		})
	}

	// Ensure we have an absolute path to the version directory.
	absPath, err := filepath.Abs(filepath.Join(rootDir, version.Path))
	if err != nil {
		return nil, err
	}

	// Parse directory entries and ignore not-exist error as webpage
	// may be generated solely from the catalog.
	files, err := os.ReadDir(absPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Lookup for any additional files that are not recorded in the
	// catalog, such as image definition.
	for _, file := range files {
		// Ignore directories.
		if file.IsDir() {
			continue
		}

		// Check if file is already found in the catalog.
		filter := func(f WebPageImageFile) bool {
			return f.Name == file.Name()
		}

		if slices.ContainsFunc(version.Files, filter) {
			continue
		}

		// Otherwise, add new file entry.
		fileInfo, err := file.Info()
		if err != nil {
			return nil, err
		}

		relPath := filepath.Join(version.Path, file.Name())
		version.Files = append(version.Files, WebPageImageFile{
			Name: file.Name(),
			Path: relPath,
			Date: formatTime(fileInfo.ModTime()),
			Size: formatSize(fileInfo.Size(), 2),
		})
	}

	// Sort files alphabetically.
	slices.SortFunc(version.Files, func(a, b WebPageImageFile) int {
		if a.Name > b.Name {
			return 0
		}

		if a.Name < b.Name {
			return -1
		}

		return 1
	})

	return &version, nil
}

// formatSize returns a human-readable string representation of the given size.
func formatSize(sizeBytes int64, precision uint) string {
	sizeStr := units.GetByteSizeString(sizeBytes, precision)

	// Read the string backwards to find the first digit
	// and add space between the number and the unit.
	for i := len(sizeStr) - 1; i >= 0; i-- {
		if unicode.IsDigit(rune(sizeStr[i])) {
			number := sizeStr[:i+1]
			unit := sizeStr[i+1:]
			return number + " " + unit
		}
	}

	return sizeStr
}

// formatTime returns a UTC time as string in format "YYYY-MM-DD (hh:mm)".
func formatTime(time time.Time) string {
	return time.UTC().Format("2006-01-02 (15:04)")
}
