package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

type DiscardOptions struct {
	RetainNum int
	ImageDirs []string
}

func NewDiscardCmd() *cobra.Command {
	var o DiscardOptions

	cmd := &cobra.Command{
		Use:     "prune <path> [flags]",
		Short:   "Prune product versions",
		Long:    "Prune product versions except for latest retaining only the specific number of latest ones.",
		GroupID: "main",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(args)
		},
	}

	cmd.PersistentFlags().IntVar(&o.RetainNum, "retain", 10, "Number of product versions to retain")
	cmd.PersistentFlags().StringSliceVarP(&o.ImageDirs, "image-dir", "d", []string{"images"}, "Image directory (relative to path argument)")

	return cmd
}

func (o *DiscardOptions) Run(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("argument %q is required and cannot be empty", "path")
	}

	if o.RetainNum < 1 {
		return fmt.Errorf("at least 1 product version must be retained")
	}

	for _, dir := range o.ImageDirs {
		err := pruneStreamProductVersions(args[0], dir, o.RetainNum)
		if err != nil {
			return err
		}
	}

	return nil
}

// pruneStreamProductVersions reads contents of index.json, finds the existing streams,
// and prunes the old-enough product versions.
func pruneStreamProductVersions(rootDir string, streamName string, retain int) error {
	jsonPath := filepath.Join(rootDir, "streams", "v1", fmt.Sprintf("%s.json", streamName))

	stream, err := shared.ReadJSONFile(jsonPath, &stream.ProductCatalog{})
	if err != nil {
		return err
	}

	// Find and store versions that need to be discarded.
	// First, we need to update stream's json file to ensure users do not
	// try to download non-existing images.
	var discardVersions []string

	for i, p := range stream.Products {
		productPath := filepath.Join(rootDir, streamName, p.RelPath())

		versions := shared.MapKeys(p.Versions)
		slices.Sort(versions)

		if len(versions) <= retain {
			continue
		}

		// Extract versions that need to be discarded.
		discard := slices.Delete(versions, 0, retain)
		for _, v := range discard {
			fmt.Printf("Discarding product version %q\n", fmt.Sprintf("%v[%s]", p.ID(), v))

			delete(stream.Products[i].Versions, v)

			versionPath := filepath.Join(productPath, v)
			discardVersions = append(discardVersions, versionPath)
		}
	}

	// Write new stream to a temporary json file.
	tmpFile, err := shared.WriteJSONTempFile(stream)
	if err != nil {
		return err
	}

	defer os.Remove(tmpFile)

	// Replace existing stream json file.
	err = os.Rename(tmpFile, jsonPath)
	if err != nil {
		return err
	}

	// Set read permissions.
	err = os.Chmod(jsonPath, 0644)
	if err != nil {
		return err
	}

	// Remove old versions.
	// TODO: How to handle errors? If image is removed from the index, and removal of the image
	// fails, the next time we rebuild the index it will be again included in the index.
	for _, v := range discardVersions {
		_ = os.RemoveAll(v)
	}

	return nil
}