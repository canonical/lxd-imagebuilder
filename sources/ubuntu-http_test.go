package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetLatestCoreBaseImage(t *testing.T) {
	release, err := getLatestCoreBaseImage("https://images.lxd.canonical.com/images", "alpine", "edge", "amd64")
	require.NoError(t, err)
	require.NotEmpty(t, release)
}
