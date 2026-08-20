package sources

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/lxd/shared/osarch"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/go-archive"
)

type docker struct {
	common
}

func dockerArchitecture(arch string) (string, string) {
	archID, err := osarch.ArchitectureId(arch)
	if err != nil {
		return arch, ""
	}

	switch archID {
	case osarch.ARCH_64BIT_INTEL_X86:
		return "amd64", ""
	case osarch.ARCH_32BIT_INTEL_X86:
		return "386", ""
	case osarch.ARCH_64BIT_ARMV8_LITTLE_ENDIAN:
		return "arm64", "v8"
	case osarch.ARCH_32BIT_ARMV7_LITTLE_ENDIAN:
		return "arm", "v7"
	case osarch.ARCH_32BIT_ARMV6_LITTLE_ENDIAN:
		return "arm", "v6"
	case osarch.ARCH_64BIT_POWERPC_LITTLE_ENDIAN:
		return "ppc64le", ""
	case osarch.ARCH_64BIT_S390_BIG_ENDIAN:
		return "s390x", ""
	case osarch.ARCH_64BIT_RISCV_LITTLE_ENDIAN:
		return "riscv64", ""
	default:
		return arch, ""
	}
}

// Run downloads and unpacks a docker image.
func (s *docker) Run() error {
	absRootfsDir, err := filepath.Abs(s.rootfsDir)
	if err != nil {
		return fmt.Errorf("Failed to get absolute path of %s: %w", s.rootfsDir, err)
	}

	var nameOpts []name.Option

	registryBase := os.Getenv("DOCKER_REGISTRY_BASE")
	if registryBase != "" {
		if strings.Contains(registryBase, "://") {
			u, err := url.Parse(registryBase)
			if err != nil {
				return fmt.Errorf("Failed to parse DOCKER_REGISTRY_BASE: %w", err)
			}

			registryBase = u.Host
			if u.Scheme == "http" {
				nameOpts = append(nameOpts, name.Insecure)
			}
		}

		nameOpts = append(nameOpts, name.WithDefaultRegistry(registryBase))
	}

	ref, err := name.ParseReference(s.definition.Source.URL, nameOpts...)
	if err != nil {
		return fmt.Errorf("Failed to parse image reference %q: %w", s.definition.Source.URL, err)
	}

	var remoteOpts []remote.Option

	if s.ctx != nil {
		remoteOpts = append(remoteOpts, remote.WithContext(s.ctx))
	}

	registryUser := os.Getenv("DOCKER_REGISTRY_BASE_USER")
	registryPass := os.Getenv("DOCKER_REGISTRY_BASE_PASS")
	if registryUser != "" || registryPass != "" {
		remoteOpts = append(remoteOpts, remote.WithAuth(&authn.Basic{
			Username: registryUser,
			Password: registryPass,
		}))
	}

	arch, variant := dockerArchitecture(s.definition.Image.Architecture)
	remoteOpts = append(remoteOpts, remote.WithPlatform(v1.Platform{
		Architecture: arch,
		Variant:      variant,
		OS:           "linux",
	}))

	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return fmt.Errorf("Failed to fetch image: %w", err)
	}

	err = os.MkdirAll(absRootfsDir, 0755)
	if err != nil {
		return fmt.Errorf("Failed to create rootfs directory %s: %w", absRootfsDir, err)
	}

	reader := mutate.Extract(img)
	defer reader.Close()

	tarOptions := &archive.TarOptions{
		ExcludePatterns: []string{"dev/"},
	}

	err = archive.Untar(reader, absRootfsDir, tarOptions)
	if err != nil {
		return fmt.Errorf("Failed to unpack image: %w", err)
	}

	return nil
}
