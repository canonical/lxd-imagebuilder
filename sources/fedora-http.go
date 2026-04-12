package sources

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	lxdShared "github.com/canonical/lxd/shared"

	"github.com/canonical/lxd-imagebuilder/shared"
)

type fedora struct {
	common
}

// Run downloads a container base image and unpacks it and its layers.
func (s *fedora) Run() error {
	base := "Fedora-Container-Base-Generic"
	baseURL := fmt.Sprintf("%s/packages/%s", s.definition.Source.URL, base)

	// Get latest build
	build, err := s.getLatestBuild(baseURL, s.definition.Image.Release)
	if err != nil {
		return fmt.Errorf("Failed to get latest build: %w", err)
	}

	fname := fmt.Sprintf("%s-%s-%s.%s.%s", base, s.definition.Image.Release, build, s.definition.Image.ArchitectureMapped, "oci.tar.xz")

	// Download image
	sourceURL := fmt.Sprintf("%s/%s/%s/images/%s", baseURL, s.definition.Image.Release, build, fname)

	fpath, err := s.DownloadHash(s.definition.Image, sourceURL, "", nil)
	if err != nil {
		return fmt.Errorf("Failed to download %q: %w", sourceURL, err)
	}

	s.logger.WithField("file", filepath.Join(fpath, fname)).Info("Unpacking image")

	// Unpack the OCI image.
	ociDir, err := os.MkdirTemp(s.getTargetDir(), "oci.")
	if err != nil {
		return fmt.Errorf("Failed to create OCI path: %q: %w", ociDir, err)
	}

	err = os.Mkdir(filepath.Join(ociDir, "image"), 0755)
	if err != nil {
		return fmt.Errorf("Failed to create OCI path: %q: %w", filepath.Join(ociDir, "image"), err)
	}

	err = shared.Unpack(filepath.Join(fpath, fname), filepath.Join(ociDir, "image"))
	if err != nil {
		return fmt.Errorf("Failed to unpack %q: %w", filepath.Join(fpath, fname), err)
	}

	// Extract the image to a temporary path.
	err = os.Mkdir(filepath.Join(ociDir, "content"), 0755)
	if err != nil {
		return fmt.Errorf("Failed to create OCI path: %q: %w", filepath.Join(ociDir, "content"), err)
	}

	_, err = lxdShared.RunCommandContext(s.ctx, "umoci", "unpack", "--keep-dirlinks", "--image", fmt.Sprintf("%s:fedora:%s", filepath.Join(ociDir, "image"), s.definition.Image.Release), filepath.Join(ociDir, "content"))
	if err != nil {
		return fmt.Errorf("Failed to run umoci: %w", err)
	}

	// Transfer the content.
	err = shared.RsyncLocal(s.ctx, fmt.Sprintf("%s/rootfs/", filepath.Join(ociDir, "content")), s.rootfsDir)
	if err != nil {
		return fmt.Errorf("Failed to run rsync: %w", err)
	}

	// Delete the temporary directory.
	err = os.RemoveAll(ociDir)
	if err != nil {
		return fmt.Errorf("Failed to wipe OCI directory: %w", err)
	}

	return nil
}

func (s *fedora) getLatestBuild(URL string, release string) (string, error) {
	s.logger.Infof("Getting latest build for release %q from %q", release, URL)

	var (
		resp *http.Response
		err  error
	)

	err = shared.Retry(func() error {
		resp, err = s.client.Get(fmt.Sprintf("%s/%s", URL, release))
		if err != nil {
			return fmt.Errorf("Failed to GET %q: %w", fmt.Sprintf("%s/%s", URL, release), err)
		}

		return nil
	}, 3)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read body: %w", err)
	}

	// Builds are formatted in one of two ways:
	//   - <yyyy><mm><dd>.<build_number>
	//   - <yyyy><mm><dd>.n.<build_number>
	re := regexp.MustCompile(`\d{8}\.(n\.)?\d`)

	// Find all builds
	matches := re.FindAllString(string(content), -1)

	if len(matches) == 0 {
		return "", errors.New("Unable to find latest build")
	}

	// Sort builds
	sort.Strings(matches)

	// Return latest build
	return matches[len(matches)-1], nil
}
