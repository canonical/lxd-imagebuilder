package embed

import (
	"embed"
)

//go:embed templates
var templates embed.FS

// GetTemplates returns the embedded templates as a filesystem.
func GetTemplates() embed.FS {
	return templates
}
