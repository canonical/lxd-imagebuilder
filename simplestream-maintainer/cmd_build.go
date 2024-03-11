package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

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

	cmd.PersistentFlags().StringVar(&o.StreamVersion, "stream-version", "v1", "Stream version")
	cmd.PersistentFlags().StringSliceVarP(&o.ImageDirs, "image-dir", "d", []string{"images"}, "Image directory (relative to path argument)")

	return cmd
}

// replace is a structure holding old and new path for a file replace.
type replace struct {
	OldPath string
	NewPath string
}

func (o *BuildOptions) Run(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("Argument %q is required and cannot be empty", "path")
	}

	rootDir := args[0]
	metaDir := path.Join(rootDir, "streams", o.StreamVersion)

	var replaces []replace
	index := stream.NewStreamIndex()

	// Create product catalogs by reading image directories.
	for _, streamName := range o.ImageDirs {
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

	// Main index file should be updated last, once all catalog files
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
