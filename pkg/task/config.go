// Package task offers utilities related to tasks in Reginald. These are shared
// between Reginald implementations and the plugin utilities.
package task

// A Config is the configuration of a task.
type Config struct {
	Type     string         // type of the task
	Name     string         // user-defined display name of the task
	Settings map[string]any // rest of the config settings
}
