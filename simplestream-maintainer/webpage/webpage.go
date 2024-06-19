package webpage

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/canonical/lxd-imagebuilder/embed"
	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

// WebPageImage represents webpage table entries.
type WebPageImage struct {
	Distribution         string
	Release              string
	Architecture         string
	Variant              string
	VersionPath          string
	VersionLastBuildDate string
	SupportsContainer    bool
	SupportsVM           bool
	IsStale              bool
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
func NewWebPage(catalog stream.ProductCatalog) *WebPage {
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
			template.HTML("Images hosted on this server are available in LXD through the predefined remote <code>images:</code>. For detailed instructions about LXD image management, please refer to our <a href='https://documentation.ubuntu.com/lxd/en/latest/howto/images_manage'>How to Manage Images</a> guide in the official documentation."),
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
			Distribution: product.OS,
			Release:      product.Release,
			Architecture: product.Architecture,
			Variant:      product.Variant,
		}

		slices.Sort(versionIds)
		last := versionIds[len(versionIds)-1]
		lastVersion := product.Versions[last]

		// Converts timestamp from format "YYYYMMDD_hhmm" into a prettier
		// format "YYYY-MM-DD (hh:mm)".
		timestamp, err := time.Parse("20060102_1504", last)
		if err != nil {
			image.VersionLastBuildDate = "N/A"
		} else {
			image.VersionLastBuildDate = timestamp.UTC().Format("2006-01-02 (15:04)")
			image.VersionPath = filepath.Join("/", catalog.ContentID, product.RelPath(), last)
		}

		// Image is considered stale if older than 8 days.
		if timestamp.Before(time.Now().AddDate(0, 0, -8)) {
			image.IsStale = true
		}

		// Iterate over version items and check if the image supports
		// containers and/or VMs.
		for _, item := range lastVersion.Items {
			if item.Ftype == stream.ItemTypeSquashfs {
				image.SupportsContainer = true
			}

			if item.Ftype == stream.ItemTypeDiskKVM {
				image.SupportsVM = true
			}
		}

		page.Images = append(page.Images, image)
	}

	return &page
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
