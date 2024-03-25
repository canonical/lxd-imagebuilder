package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

type BuildOptions struct {
	StreamVersion string
	ImageDirs     []string
}

func NewBuildCmd() *cobra.Command {
	var o BuildOptions

	cmd := &cobra.Command{
		Use:     "build <path> [flags]",
		Short:   "Build simplestream index on the given path",
		GroupID: "main",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(args)
		},
	}

	cmd.PersistentFlags().StringVar(&o.StreamVersion, "stream-version", "v1", "Stream version")
	cmd.PersistentFlags().StringSliceVarP(&o.ImageDirs, "image-dir", "d", []string{"images"}, "Image directory (relative to path argument)")

	return cmd
}

// replace struct holds old and new path for a file replace.
type replace struct {
	OldPath string
	NewPath string
}

func (o *BuildOptions) Run(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("Argument %q is required and cannot be empty", "path")
	}

	return buildIndex(args[0], o.StreamVersion, o.ImageDirs)
}

func buildIndex(rootDir string, streamVersion string, streamNames []string) error {
	metaDir := path.Join(rootDir, "streams", streamVersion)

	var replaces []replace
	index := stream.NewStreamIndex()

	// Ensure meta directory exists.
	err := os.MkdirAll(metaDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Create metadata directory: %w", err)
	}

	// Create product catalogs by reading image directories.
	for _, streamName := range streamNames {
		// Create product catalog from directory structure.
		catalog, err := buildProductCatalog(rootDir, streamVersion, streamName)
		if err != nil {
			return err
		}

		// Write product catalog to a temporary file that is located next
		// to the final file to ensure atomic replace. Temporary file is
		// prefixed with a dot to hide it.
		catalogPath := filepath.Join(metaDir, fmt.Sprintf("%s.json", streamName))
		catalogPathTemp := filepath.Join(metaDir, fmt.Sprintf(".%s.json.tmp", streamName))

		err = shared.WriteJSONFile(catalogPathTemp, catalog)
		if err != nil {
			return err
		}

		defer os.Remove(catalogPathTemp)

		replaces = append(replaces, replace{
			OldPath: catalogPathTemp,
			NewPath: catalogPath,
		})

		// Relative path for index.
		catalogRelPath, err := filepath.Rel(rootDir, catalogPath)
		if err != nil {
			return err
		}

		// Add index entry.
		index.AddEntry(streamName, catalogRelPath, *catalog)
	}

	// Write index to a temporary file that is located next to the
	// final file to ensure atomic replace. Temporary file is
	// prefixed with a dot to hide it.
	indexPath := filepath.Join(metaDir, "index.json")
	indexPathTemp := filepath.Join(metaDir, ".index.json.tmp")

	err = shared.WriteJSONFile(indexPathTemp, index)
	if err != nil {
		return err
	}

	defer os.Remove(indexPathTemp)

	// Index file should be updated last, once all catalog files
	// are in place.
	replaces = append(replaces, replace{
		OldPath: indexPathTemp,
		NewPath: indexPath,
	})

	// Move temporary files to final destinations.
	for _, r := range replaces {
		err := os.Rename(r.OldPath, r.NewPath)
		if err != nil {
			return err
		}

		// Set read permissions.
		err = os.Chmod(r.NewPath, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// buildProductCatalog fetches the corresponding product catalog and inserts
// missing products by traversing through the directory of the given stream.
func buildProductCatalog(rootDir string, streamVersion string, streamName string) (*stream.ProductCatalog, error) {
	// Get current product catalog (from json file).
	catalogPath := filepath.Join(rootDir, "streams", streamVersion, fmt.Sprintf("%s.json", streamName))
	catalog, err := shared.ReadJSONFile(catalogPath, &stream.ProductCatalog{})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if catalog == nil {
		catalog = stream.NewCatalog(nil)
	}

	// Get existing products (from actual directory hierarchy).
	products, err := stream.GetProducts(rootDir, streamName)
	if err != nil {
		return nil, err
	}

	_, newProducts := diffProducts(catalog.Products, products)
	for id, p := range newProducts {
		if len(p.Versions) == 0 {
			continue
		}

		productPath := filepath.Join(streamName, p.RelPath())

		_, ok := catalog.Products[id]
		if !ok {
			// If product does not exist yet, set the product value to one
			// that is fetched from the directory hierarchy. This ensures
			// that the product id and other metadata is set. However,
			// remove existing versions, as they will be repopulated below.
			product := products[id]
			product.Versions = make(map[string]stream.Version, len(p.Versions))
			catalog.Products[id] = product
		}

		for versionName := range p.Versions {

			// Create delta files before retrieving the version,
			// so that hashes are also calculated for delta files.
			err = createVCDiffFiles(rootDir, productPath, versionName)
			if err != nil {
				slog.Error("Failed to create delta file", "streamName", streamName, "product", id, "version", versionName, "error", err)
				continue
			}

			// Read the version and generate the file hashes.
			versionPath := filepath.Join(productPath, versionName)
			version, err := stream.GetVersion(rootDir, versionPath, true)
			if err != nil {
				slog.Error("Failed to get version", "streamName", streamName, "product", id, "version", versionName, "error", err)
				return nil, err
			}

			catalog.Products[id].Versions[versionName] = *version
		}
	}

	return catalog, nil
}

// createVCDiffFiles traverses through the directory of the given stream and
// creates missing delta (.vcdiff) files for any subsequent complete versions.
func createVCDiffFiles(rootDir string, productRelPath string, versionName string) error {
	productPath := filepath.Join(rootDir, productRelPath)

	// Get existing products (from actual directory hierarchy).
	product, err := stream.GetProduct(rootDir, productRelPath)
	if err != nil {
		return err
	}

	versions := shared.MapKeys(product.Versions)
	slices.Sort(versions)

	if len(versions) < 2 {
		// At least 2 versions must be available for diff.
		return nil
	}

	// Skip the oldest version because even if the .vcdiff does
	// not exist, we cannot generate it.
	for i := 1; i < len(versions); i++ {
		if versionName != "" && versions[i] != versionName {
			continue
		}

		preName := versions[i-1]
		curName := versions[i]

		version := product.Versions[curName]

		for _, item := range version.Items {
			// Vcdiff should be created only for qcow2 and squashfs files.
			if item.Ftype != stream.ItemTypeDiskKVM && item.Ftype != stream.ItemTypeSquashfs {
				continue
			}

			prefix, _ := strings.CutSuffix(item.Name, filepath.Ext(item.Name))
			suffix := "vcdiff"

			if item.Ftype == stream.ItemTypeDiskKVM {
				suffix = "qcow2.vcdiff"
			}

			vcdiff := fmt.Sprintf("%s.%s.%s", prefix, preName, suffix)
			_, ok := version.Items[vcdiff]
			if ok {
				// Delta already exists. Skip..
				slog.Debug("Delta already exists", "version", curName, "deltaBase", preName)
				continue
			}

			sourcePath := filepath.Join(productPath, preName, item.Name)
			targetPath := filepath.Join(productPath, curName, item.Name)
			outputPath := filepath.Join(productPath, curName, vcdiff)

			// Ensure source path exists.
			_, err := os.Stat(sourcePath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					// Source does not exist. Skip..
					continue
				}

				return err
			}

			err = calcVCDiff(sourcePath, targetPath, outputPath)
			if err != nil {
				return err
			}

			slog.Info("Delta generated successfully", "version", curName, "deltaBase", preName)
		}
	}

	return nil
}

func calcVCDiff(sourcePath string, targetPath string, outputPath string) error {
	bin, err := exec.LookPath("xdelta3")
	if err != nil {
		return err
	}

	// -e compress
	// -f force
	cmd := exec.Command(bin, "-e", "-s", sourcePath, targetPath, outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		_ = os.Remove(outputPath)
		return err
	}

	return nil
}

// DiffProducts is a helper function that compares two product maps and returns
// the difference between them.
func diffProducts(oldProducts map[string]stream.Product, newProducts map[string]stream.Product) (old map[string]stream.Product, new map[string]stream.Product) {
	old = make(map[string]stream.Product) // Extra (old) products.
	new = make(map[string]stream.Product) // Missing (new) products.

	// Extract new products and versions.
	for id, p := range newProducts {
		_, ok := oldProducts[id]
		if !ok {
			// Product is missing in the old catalog.
			new[id] = p
			continue
		}

		for name, v := range p.Versions {
			_, ok := oldProducts[id].Versions[name]
			if !ok {
				// Version is missing in the old catalog.
				new[id].Versions[name] = v
			}
		}
	}

	// Extract old products and versions.
	for id, p := range oldProducts {
		_, ok := newProducts[id]
		if !ok {
			// Product is missing in the new catalog.
			old[id] = p
			continue
		}

		for name, v := range p.Versions {
			_, ok := newProducts[id].Versions[name]
			if !ok {
				// Version is missing in the new catalog.
				old[id].Versions[name] = v
			}
		}
	}

	return old, new
}
