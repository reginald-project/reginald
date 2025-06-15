package builtin

import "github.com/reginald-project/reginald-sdk-go/api"

// coreManifest returns the manifest for the core plugin.
func coreManifest() api.Manifest {
	return api.Manifest{
		Name:   "builtin",
		Domain: "core",
		// TODO : Add a description.
		Description: "TODO",
		Executable:  "",
		Config:      nil,
		Commands: []api.Command{
			{
				Name:  "attend",
				Usage: "attend [options]",
				// TODO : Add a description.
				Description: "TODO",
				Aliases:     []string{"apply", "tend"},
				Config:      nil,
			},
		},
		Tasks: nil,
	}
}
