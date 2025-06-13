package plugin

import "github.com/reginald-project/reginald-sdk-go/api"

// An External is an external plugin that is not provided by the program itself.
// It implements the plugin client in Reginald for calling methods from
// the plugin executables.
type External struct {
	// manifest is the manifest for this plugin.
	manifest api.Manifest

	// loaded tells whether the executable for this plugin is loaded and started
	// up.
	loaded bool
}

// Manifest returns the loaded manifest for the plugin.
func (e *External) Manifest() api.Manifest {
	return e.manifest
}

// newExternal returns a new external plugin for the given manifest.
func newExternal(m api.Manifest) *External {
	return &External{
		manifest: m,
		loaded:   false,
	}
}
