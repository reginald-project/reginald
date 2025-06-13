package plugin

import "github.com/reginald-project/reginald-sdk-go/api"

// A Builtin is a built-in plugin provided by Reginald. It is implemented within
// the program and it must not use an external executable.
type Builtin struct {
	// manifest is the manifest for this plugin.
	manifest api.Manifest
}

// Manifest returns the loaded manifest for the plugin.
func (b *Builtin) Manifest() api.Manifest {
	return b.manifest
}

// newBuiltin returns a new built-in plugin for the given manifest.
func newBuiltin(m api.Manifest) *Builtin {
	return &Builtin{
		manifest: m,
	}
}
