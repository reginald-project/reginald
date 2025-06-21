// Copyright 2025 The Reginald Authors
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

//go:build tool

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/reginald-project/reginald/tools"
)

const versionPackage = "github.com/reginald-project/reginald/internal/version"

func main() {
	log.SetFlags(0)

	var task string

	if len(os.Args) < 2 {
		task = "reginald"
	} else {
		task = os.Args[1]
	}

	exe := os.Getenv("GO")
	if exe == "" {
		exe = "go"
	}

	self := filepath.Base(os.Args[0])
	if self == "buildtask" {
		self = "buildtask.go"
	}

	tasks := map[string]func() error{
		"reginald": func() error {
			output := os.Getenv("OUTPUT")
			if output == "" {
				output = "reginald"
			}

			if isWindows() {
				output += ".exe"
			}

			info, err := os.Stat(output)
			if err == nil && !sourceFilesLaterThan(info.ModTime()) {
				fmt.Printf("%s: `%s` is up to date.\n", self, output)

				return nil
			}

			version := os.Getenv("VERSION")

			if version == "" {
				data, err := os.ReadFile("VERSION")
				if err != nil {
					return fmt.Errorf("%w", err)
				}

				version = strings.TrimSpace(string(data))
				version += "-0.dev." + time.Now().UTC().Format("20060102150405")
				// TODO: Add build metadata if needed.
			}

			args := []string{exe, "build", "-trimpath"}
			args = append(args, strings.Fields(os.Getenv("GOFLAGS"))...)
			args = append(args, "-ldflags", "-X "+versionPackage+".buildVersion="+version)
			args = append(args, "-o", output)

			if err := tools.Run(args...); err != nil {
				return fmt.Errorf("%w", err)
			}

			return nil
		},
	}

	t, ok := tasks[task]
	if !ok {
		log.Fatalf("Don't know how to build task `%s`", task)
	}

	if err := t(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "%s: building task `%s` failed\n", self, task)
		os.Exit(1)
	}
}

func isAccessDenied(err error) bool {
	var pathError *os.PathError

	return errors.As(err, &pathError) && strings.Contains(pathError.Err.Error(), "Access is denied")
}

func isWindows() bool {
	if os.Getenv("GOOS") == "windows" {
		return true
	}

	if runtime.GOOS == "windows" {
		return true
	}

	return false
}

func sourceFilesLaterThan(t time.Time) bool {
	foundLater := false

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Ignore errors that occur when the project contains a symlink to
			// a filesystem or volume that Windows doesn't have access to.
			if path != "." && isAccessDenied(err) {
				fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				return nil
			}

			return err
		}

		if foundLater {
			return filepath.SkipDir
		}

		if len(path) > 1 && (path[0] == '.' || path[0] == '_') {
			if info.IsDir() {
				return filepath.SkipDir
			} else {
				return nil
			}
		}

		if info.IsDir() {
			if name := filepath.Base(path); name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if path == "go.mod" || path == "go.sum" ||
			(strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")) {
			if info.ModTime().After(t) {
				foundLater = true
			}
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	return foundLater
}
