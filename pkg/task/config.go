// Package task offers utilities related to tasks in Reginald. These are shared
// between Reginald implementations and the plugin utilities.
package task

// A Config is the configuration of a task.
type Config struct {
	// Type is the type of this task. It defines which task implementation is
	// called when this task is executed.
	Type string `mapstructure:"type"`

	// ID is the unique ID for this task. It most be unique.
	ID string `mapstructure:"id"`

	// Options contains the rest of the config options for the task.
	Options map[string]any `mapstructure:",remain"`
}
