// Copyright 2025 Antti Kivi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

import "github.com/anttikivi/reginald/pkg/rpp"

// A Command is an interface that the commands defined by plugins that use the
// provided plugin server should implement. The plugin server uses the values
// returned by this interface to create the required messages and the functions
// to run the command.
type Command interface {
	// Name returns the name of the command as it should be written by the user
	// when they run the command. It must not match any existing commands either
	// within Reginald or other plugins.
	Name() string

	// UsageLine is the one-line usage synopsis of the command.
	UsageLine() string

	// Configs returns the config options for this Command. The config options
	// should also define the command-line flags of this command.
	Configs() []rpp.ConfigEntry

	// Run runs the command.
	Run(cfg []rpp.ConfigEntry) error
}
