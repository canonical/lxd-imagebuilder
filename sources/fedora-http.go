package sources

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/lxd-imagebuilder/shared"
)

type fedora struct {
	commonRHEL
}

// Run downloads a container base image and unpacks it and its layers.
func (s *fedora) Run() error {
	// For backwards compatibility, fallback to manual URL construction
	// when the release version is equal or less than 40.
	relNum, err := strconv.Atoi(s.definition.Image.Release)
	if err == nil && relNum <= 40 {
		baseURL := fmt.Sprintf("%s/packages/Fedora-Container-Base", s.definition.Source.URL)

		// Get latest build
		build, err := s.getLatestBuild(baseURL, s.definition.Image.Release)
		if err != nil {
			return fmt.Errorf("Failed to get latest build: %w", err)
		}

		fname := fmt.Sprintf("Fedora-Container-Base-%s-%s.%s.tar.xz", s.definition.Image.Release, build, s.definition.Image.ArchitectureMapped)

		// Download image
		sourceURL := fmt.Sprintf("%s/%s/%s/images/%s", baseURL, s.definition.Image.Release, build, fname)

		fpath, err := s.DownloadHash(s.definition.Image, sourceURL, "", nil)
		if err != nil {
			return fmt.Errorf("Failed to download %q: %w", sourceURL, err)
		}

		s.logger.WithField("file", filepath.Join(fpath, fname)).Info("Unpacking image")

		// Unpack the base image
		err = shared.Unpack(filepath.Join(fpath, fname), s.rootfsDir)
		if err != nil {
			return fmt.Errorf("Failed to unpack %q: %w", filepath.Join(fpath, fname), err)
		}

		s.logger.Info("Unpacking layers")

		// Unpack the rest of the image (/bin, /sbin, /usr, etc.)
		err = s.unpackLayers(s.rootfsDir)
		if err != nil {
			return fmt.Errorf("Failed to unpack: %w", err)
		}

		return nil
	}

	// Otherwise, get the ISO image URL from the available releases.
	sourceURL, checksum, err := s.getDownloadURL(s.definition.Image.Release, s.definition.Image.Variant, s.definition.Image.ArchitectureMapped, ".iso")
	if err != nil {
		return err
	}

	if checksum == "" {
		return fmt.Errorf("Checksum not found for %q", sourceURL)
	}

	fpath, err := s.DownloadHash(s.definition.Image, sourceURL, "", nil)
	if err != nil {
		return fmt.Errorf("Failed to download %q: %w", sourceURL, err)
	}

	fname := filepath.Join(fpath, filepath.Base(sourceURL))

	err = shared.VerifyChecksum(fname, checksum, sha256.New())
	if err != nil {
		return fmt.Errorf("Failed to verify checksum: %w", err)
	}

	s.logger.WithField("file", fname).Info("Unpacking image")

	// Unpack the base image
	err = s.unpackISO(fname, s.rootfsDir, s.isoRunner)
	if err != nil {
		return fmt.Errorf("Failed to unpack %q: %w", fname, err)
	}

	return nil
}

func (s *fedora) unpackLayers(rootfsDir string) error {
	// Read manifest file which contains the path to the layers
	file, err := os.Open(filepath.Join(rootfsDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", filepath.Join(rootfsDir, "manifest.json"), err)
	}

	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("Failed to read file %q: %w", file.Name(), err)
	}

	// Structure of the manifest excluding RepoTags
	var manifests []struct {
		Layers []string
		Config string
	}

	err = json.Unmarshal(data, &manifests)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal JSON data: %w", err)
	}

	pathsToRemove := []string{
		filepath.Join(rootfsDir, "manifest.json"),
		filepath.Join(rootfsDir, "repositories"),
	}

	// Unpack tarballs (or layers) which contain the rest of the rootfs, and
	// remove files not relevant to the image.
	for _, manifest := range manifests {
		for _, layer := range manifest.Layers {
			s.logger.WithField("file", filepath.Join(rootfsDir, layer)).Info("Unpacking layer")

			err := shared.Unpack(filepath.Join(rootfsDir, layer), rootfsDir)
			if err != nil {
				return fmt.Errorf("Failed to unpack %q: %w", filepath.Join(rootfsDir, layer), err)
			}

			pathsToRemove = append(pathsToRemove,
				filepath.Join(rootfsDir, filepath.Dir(layer)))
		}

		pathsToRemove = append(pathsToRemove, filepath.Join(rootfsDir, manifest.Config))
	}

	// Clean up /tmp since there are unnecessary files there
	files, err := filepath.Glob(filepath.Join(rootfsDir, "tmp", "*"))
	if err != nil {
		return fmt.Errorf("Failed to find matching files: %w", err)
	}

	pathsToRemove = append(pathsToRemove, files...)

	// Clean up /root since there are unnecessary files there
	files, err = filepath.Glob(filepath.Join(rootfsDir, "root", "*"))
	if err != nil {
		return fmt.Errorf("Failed to find matching files: %w", err)
	}

	pathsToRemove = append(pathsToRemove, files...)

	for _, f := range pathsToRemove {
		os.RemoveAll(f)
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
		resp, err = http.Get(fmt.Sprintf("%s/%s", URL, release))
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

// getDownloadURL fetches JSON representation of current releases and
// extracts the download URL matching the given release, variant, arch,
// and filetype (.tar.xz, .iso, etc.).
func (s *fedora) getDownloadURL(release string, variant string, arch string, fileType string) (url string, checksum string, err error) {
	releasesURL := "https://fedoraproject.org/releases.json"
	s.logger.Infof("Querying %q for list of available releases", releasesURL)

	// Fetch available releases.
	resp, err := http.Get(releasesURL)
	if err != nil {
		return "", "", fmt.Errorf("Failed to fetch releases from %q: %w", releasesURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("Unexpected status code %q from %q: %w", resp.StatusCode, releasesURL, err)
	}

	// Parse JSON response.
	var result []struct {
		Release    string `json:"version"`
		Arch       string `json:"arch"`
		Variant    string `json:"variant"`
		Subvariant string `json:"subvariant"`
		URL        string `json:"link"`
		SHA256     string `json:"sha256"`
		SizeBytes  string `json:"size"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", "", fmt.Errorf("Failed to parse JSON response: %w", err)
	}

	// Variant mapping.
	// Default to variant "Container" and subvariant "Container_Base".
	var subvariant string
	if variant == "default" || variant == "cloud" {
		variant = "Server"
		subvariant = "Server"
	}

	// Iterate over response items and find the matching image.
	for _, item := range result {
		if item.Release != release || item.Arch != arch || item.Variant != variant {
			continue
		}

		if subvariant != "" && item.Subvariant != subvariant {
			continue
		}

		if !strings.HasSuffix(item.URL, fileType) {
			continue
		}

		// Matching URL found.
		return item.URL, item.SHA256, nil
	}

	return "", "", fmt.Errorf("Failed to find download URL for release %q, variant %q, arch %q, and file type %q", release, variant, arch, fileType)
}

func (s *fedora) isoRunner(gpgkeys string) error {
	err := shared.RunScript(s.ctx, fmt.Sprintf(`#!/bin/sh
set -eux

# Create required files
touch /etc/mtab /etc/fstab

dnf_args=""
mkdir -p /etc/yum.repos.d

# Add cdrom repo
cat <<- EOF > /etc/yum.repos.d/cdrom.repo
[cdrom]
name=Install CD-ROM
baseurl=file:///mnt/cdrom
enabled=0
EOF

GPG_KEYS="%s"

gpg_keys_official="file:///etc/pki/rpm-gpg/RPM-GPG-KEY-fedora-%[2]s-primary"

if [ -n "${GPG_KEYS}" ]; then
	echo gpgcheck=1 >> /etc/yum.repos.d/cdrom.repo
	echo gpgkey=${gpg_keys_official} ${GPG_KEYS} >> /etc/yum.repos.d/cdrom.repo
else
	echo gpgcheck=0 >> /etc/yum.repos.d/cdrom.repo
fi

dnf_args="--disablerepo=* --enablerepo=cdrom"

# Newest install.img doesnt have rpm installed,
# so first install rpm.
if ! command -v rpmkeys; then
	cd /mnt/cdrom/Packages
	dnf ${dnf_args} -y install rpm
fi

pkgs="basesystem fedora-release dnf"

# Create a minimal rootfs
mkdir /rootfs
dnf ${dnf_args} --installroot=/rootfs --releasever=%[2]s -y install ${pkgs}
rm -rf /rootfs/var/cache/yum
rm -rf /rootfs/var/cache/dnf
rm -rf /etc/yum.repos.d/cdrom.repo
# Remove all files in mnt packages
rm -rf /mnt/cdrom
`, gpgkeys, s.definition.Image.Release))

	if err != nil {
		return fmt.Errorf("Failed to run script: %w", err)
	}

	return nil
}
