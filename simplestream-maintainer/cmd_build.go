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
	DiffProducts  bool
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

	cmd.PersistentFlags().BoolVar(&o.DiffProducts, "diff-products", false, "Create missing .vcdiff files")
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

	return rebuildIndex(args[0], o.StreamVersion, o.ImageDirs, o.DiffProducts)
}

func rebuildIndex(rootDir string, streamVersion string, streamNames []string, diffProducts bool) error {
	metaDir := path.Join(rootDir, "streams", streamVersion)

	var replaces []replace
	index := stream.NewStreamIndex()

	// Create product catalogs by reading image directories.
	for _, streamName := range streamNames {
		if diffProducts {
			err := createVCDiffFiles(rootDir, streamName)
			if err != nil {
				return err
			}
		}

		// Create product catalog from directory structure.
		catalog, err := stream.GetProductCatalog(rootDir, streamName)
		if err != nil {
			return err
		}

		// Create temporary catalog json file.
		catalogPath := filepath.Join(metaDir, fmt.Sprintf("%s.json", streamName))
		catalogPathTemp, err := shared.WriteJSONTempFile(catalog)
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

	indexPath := filepath.Join(metaDir, "index.json")
	indexPathTemp, err := shared.WriteJSONTempFile(index)
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

	// Ensure meta directory exists.
	err = os.MkdirAll(metaDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Create metadata directory: %w", err)
	}

	// Replace all files.
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

// createVCDiffFiles traverses through the directory of the given stram and
// creates missing VCDiff (.vcdiff) files for any subsequent versions.
func createVCDiffFiles(rootDir string, streamName string) error {
	// Get existing products (from actual directory hierarchy).
	products, err := stream.GetProducts(rootDir, streamName, false)
	if err != nil {
		return err
	}

	for _, p := range products {
		versions := shared.MapKeys(p.Versions)
		if len(versions) < 2 {
			// At least 2 versions must be available for diff.
			continue
		}

		productPath := filepath.Join(rootDir, streamName, p.RelPath())
		slices.Sort(versions)

		// Skip the oldest version because even if the .vcdiff does
		// not exist, we cannot generate it.
		for i := 1; i < len(versions); i++ {
			preName := versions[i-1]
			curName := versions[i]

			version := p.Versions[curName]

			for _, item := range version.Items {
				// Vcdiff should be created only for qcow2 and squashfs files.
				if item.Ftype != stream.ItemType_DiskKVM && item.Ftype != stream.ItemType_Squshfs {
					continue
				}

				prefix, _ := strings.CutSuffix(item.Name, filepath.Ext(item.Name))
				suffix := "vcdiff"

				if item.Ftype == stream.ItemType_DiskKVM {
					suffix = "qcow2.vcdiff"
				}

				vcdiff := fmt.Sprintf("%s.%s.%s", prefix, preName, suffix)
				_, ok := version.Items[vcdiff]
				if ok {
					// Delta already exists. Skip..
					slog.Debug("Delta already exists", "productId", p.ID(), "version", curName, "deltaBase", preName)
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

				slog.Debug("Delta generated successfully", "productId", p.ID(), "version", curName, "deltaBase", preName)
			}
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
