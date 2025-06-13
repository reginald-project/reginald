package cli

// An ExitError is an error returned by the CLI that wraps an error that is
// causing the program to exit and associates an exit code with it. The program
// will return the exit code once it ends its execution.
type ExitError struct {
	// Code is the exit code associated with this error. It will be used by
	// the program as the exit code it returns to the caller.
	Code int
	err  error
}

// Error returns the value of e as a string. This function implements the error
// interface for ExitError.
func (e *ExitError) Error() string {
	return e.err.Error()
}
