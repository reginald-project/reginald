package plugin

// Task is a task that Reginald can run. The task implementation is resolved by
// the applying commands from either Reginald itself or plugins.
type Task interface {
	// Name returns the name of the task type as it should be written by
	// the user when they specify it in, for example, their configuration. It
	// must not match any existing tasks either within Reginald or other
	// plugins.
	Name() string
}
