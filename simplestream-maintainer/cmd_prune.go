package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/lxd-imagebuilder/shared"
	"github.com/canonical/lxd-imagebuilder/simplestream-maintainer/stream"
)

type DiscardOptions struct {
	Dangling  bool
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

	cmd.PersistentFlags().BoolVar(&o.Dangling, "dangling", false, "Remove dangling product versions (not referenced from product catalog)")
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
		if o.Dangling {
			err := pruneDanglingProductVersions(args[0], dir)
			if err != nil {
				return err
			}
		}

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

// pruneDanglingProductVersions traverses through the stream directory structure
// and prunes the dangling product versions.
func pruneDanglingProductVersions(rootDir string, streamName string) error {
	// Get existing products (from actual directory hierarchy).
	existingProducts, err := stream.GetProducts(rootDir, streamName, false)
	if err != nil {
		return err
	}

	// Get current products (from stream json file).
	jsonPath := filepath.Join(rootDir, "streams", "v1", fmt.Sprintf("%s.json", streamName))
	stream, err := shared.ReadJSONFile(jsonPath, &stream.ProductCatalog{})
	if err != nil {
		return err
	}

	// Remove product versions that are not referenced, but exists.
	// Note: Only versions that are at least 1 day old are removed to avoid
	// removing versions that are being currently uploaded.
	for key, ep := range existingProducts {
		sp, ok := stream.Products[key]
		if !ok {
			// TODO: If product is not found in the stream and is older than 1 day,
			// remove entire dangling product.
			continue
		}

		productPath := filepath.Join(rootDir, streamName, ep.RelPath())

		for epv := range ep.Versions {
			_, ok := sp.Versions[epv]
			if ok {
				fmt.Printf("Version %s[%s] is referenced\n", ep.ID(), epv)
				// Version is referenced, nothing to do.
				continue
			}

			versionPath := filepath.Join(productPath, epv)

			info, err := os.Stat(versionPath)
			if err != nil {
				return err
			}

			// Remove the version only if the difference between the current time
			// and last modification is more then 1 day, to avoid accidentally
			// removing version that is just being uploaded.
			if time.Since(info.ModTime()) > 24*time.Hour {
				fmt.Printf("Discrading dangling product version: %s[%s]\n", ep.ID(), epv)
				_ = os.RemoveAll(versionPath)
			}
		}
	}

	return nil
}
