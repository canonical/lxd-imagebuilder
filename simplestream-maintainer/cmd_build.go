package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

type buildOptions struct {
	global *globalOptions

	StreamVersion string
	ImageDirs     []string
	Workers       int
}

func (o *buildOptions) NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build <path> [flags]",
		Short:   "Build simplestream index on the given path",
		GroupID: "main",
		RunE:    o.Run,
	}

	cmd.PersistentFlags().StringVar(&o.StreamVersion, "stream-version", "v1", "Stream version")
	cmd.PersistentFlags().StringSliceVarP(&o.ImageDirs, "image-dir", "d", []string{"images"}, "Image directory (relative to path argument)")
	cmd.PersistentFlags().IntVar(&o.Workers, "workers", max(runtime.NumCPU()/2, 1), "Maximum number of concurrent operations")

	return cmd
}

func (o *buildOptions) Run(_ *cobra.Command, args []string) error {
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("Argument %q is required and cannot be empty", "path")
	}

	return buildIndex(o.global.ctx, args[0], o.StreamVersion, o.ImageDirs, o.Workers)
}

// replace struct holds old and new path for a file replace.
type replace struct {
	OldPath string
	NewPath string
}

func buildIndex(ctx context.Context, rootDir string, streamVersion string, streamNames []string, workers int) error {
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
		catalog, err := buildProductCatalog(ctx, rootDir, streamVersion, streamName, workers)
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

// buildProductCatalog compares the existing product catalog and actual products on
// the disk. For missing any new version, hashes are calculated and compared against
// the checksums file. Based on the final catalog (that contains only valid version)
// missing delta files are generated. Finally the catalog is returned.
//
// Note: Workers limit the maximum number of concurent tasks when calulcating hashes
// and delta files.
func buildProductCatalog(ctx context.Context, rootDir string, streamVersion string, streamName string, workers int) (*stream.ProductCatalog, error) {
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

	var wg sync.WaitGroup
	var mutex sync.Mutex // To safely update the catalog.Products map

	// Ensure at least 1 worker is spawned.
	if workers < 1 {
		workers = 1
	}

	// Job queue.
	jobs := make(chan func(), workers)
	defer close(jobs)

	// Create new pool of workers.
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}

					job()
				}
			}
		}()
	}

	// Extract new (unreferenced products and product versions) and add them
	// to the catalog.
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

			// Lock before updating, as another gorotine may be accessing it.
			mutex.Lock()
			catalog.Products[id] = product
			mutex.Unlock()
		}

		for versionName := range p.Versions {
			// Add a job for processing a new version.
			wg.Add(1)
			jobs <- func() {
				defer wg.Done()

				// Read the version and generate the file hashes.
				versionPath := filepath.Join(productPath, versionName)
				version, err := stream.GetVersion(rootDir, versionPath, true)
				if err != nil {
					slog.Error("Failed to get version", "streamName", streamName, "product", id, "version", versionName, "error", err)
					return
				}

				// Verify items checksums if checksum file is present
				// within the version.
				if version.Checksums != nil {
					for _, item := range version.Items {
						checksum := version.Checksums[item.Name]

						// Ignore verification, if the checksum for the delta
						// file does not exist. This is because the delta file
						// is generated after the checksums file is created.
						if !ok && (item.Ftype == stream.ItemTypeDiskKVMDelta || item.Ftype == stream.ItemTypeSquashfsDelta) {
							continue
						}

						// Verify checksum.
						if checksum != item.SHA256 {
							slog.Error("Checksum mismatch", "streamName", streamName, "product", id, "version", versionName, "item", item.Name)
							return
						}
					}
				}

				mutex.Lock()
				catalog.Products[id].Versions[versionName] = *version
				mutex.Unlock()

				slog.Info("New version added to the product catalog", "streamName", streamName, "product", id, "version", versionName)
			}
		}
	}

	// Wait for all workers to finish to ensure the final catalog contains
	// all valid product versions.
	wg.Wait()

	// Build delta files after all new versions are added to the catalog.
	// This way we can determine which versions are valid for delta files.
	//
	// Traverse through the products. For each product iterate over versions
	// and find items that are valid for delta files. If a delta file already
	// exists, ensure that the catalog contains its file hash. If a delta file
	// does not exist, create it and update the catalog with the new file hash.
	for id, product := range catalog.Products {
		productRelPath := filepath.Join(streamName, product.RelPath())

		versions := shared.MapKeys(product.Versions)
		slices.Sort(versions)

		if len(versions) < 2 {
			// At least 2 versions must be available for delta.
			continue
		}

		// Skip the oldest version because even if the .vcdiff does
		// not exist, we cannot generate it.
		for i := 1; i < len(versions); i++ {
			sourceVerName := versions[i-1]
			targetVerName := versions[i]
			targetVersion := product.Versions[targetVerName]

			for _, item := range targetVersion.Items {
				// Delta should be created only for qcow2 and squashfs files.
				if item.Ftype != stream.ItemTypeDiskKVM && item.Ftype != stream.ItemTypeSquashfs {
					continue
				}

				wg.Add(1)
				jobs <- func() {
					defer wg.Done()

					// Evaluate delta file name.
					prefix, _ := strings.CutSuffix(item.Name, filepath.Ext(item.Name))
					suffix := "vcdiff"

					if item.Ftype == stream.ItemTypeDiskKVM {
						suffix = "qcow2.vcdiff"
					}

					deltaName := fmt.Sprintf("%s.%s.%s", prefix, sourceVerName, suffix)
					deltaItem, deltaExists := targetVersion.Items[deltaName]

					// Generate delta file if it does not already exist.
					if !deltaExists {
						sourcePath := filepath.Join(rootDir, productRelPath, sourceVerName, item.Name)
						targetPath := filepath.Join(rootDir, productRelPath, targetVerName, item.Name)
						outputPath := filepath.Join(rootDir, productRelPath, targetVerName, deltaName)

						// Ensure source path exists.
						_, err := os.Stat(sourcePath)
						if err != nil {
							if errors.Is(err, os.ErrNotExist) {
								// Source does not exist. Skip..
								return
							}

							slog.Error("Failed to read base delta file", "product", id, "version", targetVerName, "item", item.Name, "deltaBase", sourceVerName, "error", err)
							return
						}

						// -e compress
						// -f force
						cmd := exec.CommandContext(ctx, "xdelta3", "-e", "-s", sourcePath, targetPath, outputPath)
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr

						err = cmd.Run()
						if err != nil {
							slog.Error("Failed creating delta file", "product", id, "version", targetVerName, "item", deltaName, "deltaBase", sourceVerName, "error", err)
							_ = os.Remove(outputPath)
							return
						}

						slog.Info("Delta generated successfully", "product", id, "version", targetVerName, "item", deltaName, "deltaBase", sourceVerName)
					}

					// If delta file exists but is missing a hash in the catalog,
					// or was just generated, calculate it's hash and add it to
					// the catalog.
					if !deltaExists || deltaItem.SHA256 == "" {
						deltaRelPath := filepath.Join(productRelPath, targetVerName, deltaName)
						deltaItem, err := stream.GetItem(rootDir, deltaRelPath, true)
						if err != nil {
							slog.Error("Failed to get existing delta item", "product", id, "version", targetVerName, "item", deltaName, "error", err)
							return
						}

						// Append delta file hash to the version checksums
						// file if it exists.
						_, ok := targetVersion.Checksums[deltaName]
						if !ok && len(targetVersion.Checksums) > 0 {
							// Append new item to the checksums file.
							checksumFile := filepath.Join(rootDir, productRelPath, targetVerName, stream.FileChecksumSHA256)
							err := shared.AppendToFile(checksumFile, fmt.Sprintf("%s  %s\n", deltaItem.SHA256, deltaName))
							if err != nil {
								slog.Error("Failed to update checksums file", "product", id, "version", targetVerName, "error", err)
								return
							}

							// Update version checksums map.
							mutex.Lock()
							catalog.Products[id].Versions[targetVerName].Checksums[deltaName] = deltaItem.SHA256
							mutex.Unlock()
						}

						// Include delta item with hashes in the catalog.
						mutex.Lock()
						catalog.Products[id].Versions[targetVerName].Items[deltaName] = *deltaItem
						mutex.Unlock()
					}
				}
			}
		}
	}

	// Wait for all goroutines to finish.
	wg.Wait()

	return catalog, nil
}

// DiffProducts is a helper function that compares two product maps and returns
// the difference between them.
func diffProducts(oldProducts map[string]stream.Product, newProducts map[string]stream.Product) (map[string]stream.Product, map[string]stream.Product) {
	findMissing := func(mapOld map[string]stream.Product, mapNew map[string]stream.Product) map[string]stream.Product {
		missing := make(map[string]stream.Product)

		for id, p := range mapNew {
			_, ok := mapOld[id]
			if !ok {
				// Product is missing in the old map.
				missing[id] = p
				continue
			}

			// Ensure we are not modifying product's nested map directly.
			versions := make(map[string]stream.Version, len(p.Versions))

			for name, v := range p.Versions {
				_, ok := mapOld[id].Versions[name]
				if !ok {
					// Version exists in the old map.
					versions[name] = v
				}
			}

			if len(versions) > 0 {
				p.Versions = versions
				missing[id] = p
			}
		}
		return missing
	}

	new := findMissing(oldProducts, newProducts)
	old := findMissing(newProducts, oldProducts)

	return old, new
}
