// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/pkg/task"
)

// Config is the parsed configuration of the program run. There should be only
// one effective Config per run.
type Config struct {
	// Color tells whether colors should be enabled in the user output.
	Color bool `mapstructure:"color"`

	// ConfigFile is the absolute path to the config file in use.
	ConfigFile string `mapstructure:"config-file"`

	Logging logging.Config `mapstructure:"logging"`

	// PluginDir is the directory where Reginald looks for the plugins.
	PluginDir string `mapstructure:"plugin-dir"`

	// Quiet tells the program to suppress all other output than errors.
	Quiet bool `mapstructure:"quiet"`

	// Tasks contains tasks and the configs for them as given in the config
	// file.
	Tasks []task.Config `mapstructure:"tasks"`

	// Verbose tells the program to print more verbose output.
	Verbose bool `mapstructure:"verbose"`

	// Plugins contains the rest of the config options which should only be
	// plugin-defined options.
	Plugins map[string]any `mapstructure:",remain"`
}

// Equal reports if the Config that d points to is equal to the Config that c
// points to.
func (c *Config) Equal(d *Config) bool {
	if c == d {
		return true
	}

	if c == nil {
		return d == nil
	}

	if d == nil {
		return c == nil
	}

	if len(c.Tasks) != len(d.Tasks) {
		return false
	}

	for i, t := range c.Tasks {
		u := d.Tasks[i]

		if t.Type != u.Type || t.ID != u.ID {
			return false
		}

		for k, a := range t.Options {
			if b, ok := u.Options[k]; !ok || a != b {
				return false
			}
		}
	}

	return c.Color == d.Color && c.ConfigFile == d.ConfigFile && c.Logging == d.Logging &&
		c.PluginDir == d.PluginDir &&
		c.Quiet == d.Quiet &&
		c.Verbose == d.Verbose
}
