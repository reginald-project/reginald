// Package builtin provides the built-in plugins of Reginald. They define
// the commands and tasks that are included with Reginald.
package builtin

import "github.com/reginald-project/reginald-sdk-go/api"

// Manifests returns the plugin manifests for the built-in plugins.
func Manifests() []api.Manifest {
	return []api.Manifest{
		{
			Name:        "builtin",
			Domain:      "builtin",
			Description: "TODO",
			Executable:  "",
		},
	}
}
