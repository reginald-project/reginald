// Package task defines the public interface for creating tasks for Reginald.
package task

// Task is a task that Reginald can run. The task implementation is resolved by
// the applying commands from either Reginald itself or plugins.
type Task interface {
	// Rules returns the configuration validation rules for the configuration
	// settings of this Task. As the settings are provided as a map, Reginald
	// uses the rules to validate the configuration entries for the tasks of
	// this type.
	//
	// TODO: Maybe the rules should have some other type.
	Rules() map[string]any
}

// A Config is the configuration of a task.
type Config struct {
	Type     string         // type of the task
	Name     string         // user-defined display name of the task
	Settings map[string]any // rest of the config settings
}
