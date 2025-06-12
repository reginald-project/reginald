// Package plugin implements the plugin client of Reginald.
package plugin

// A Plugin is a plugin that Reginald recognizes.
type Plugin interface {
	Name() string
}
