// Package kubeonramp holds the data files that are built into the binary.
//
// The embed directive can't reach parent directories, so this package lives
// at the module root next to profiles/ and catalog/ (docs/DECISIONS.md, D6).
package kubeonramp

import "embed"

// Profiles holds every profiles/*.yaml file.
//
//go:embed profiles/*.yaml
var Profiles embed.FS

// Catalog is the content of catalog/tools.yaml.
//
//go:embed catalog/tools.yaml
var Catalog []byte
