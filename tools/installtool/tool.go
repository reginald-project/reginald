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

//go:build tool

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/reginald-project/reginald/tools/run"
)

func main() {
	log.SetFlags(0)

	var tool string

	if len(os.Args) < 2 {
		log.Fatal("No tool given")
	} else {
		tool = os.Args[1]
	}

	flagSet := flag.NewFlagSet("installtool", flag.ExitOnError)
	exe := flagSet.String("go", "go", "path to the Go executable")
	force := flagSet.Bool("f", false, "reinstall the tool if it is already installed")
	flagSet.Usage = func() {
		fmt.Fprintln(flagSet.Output(), "Usage: installtool [flags]")
		flagSet.PrintDefaults()
	}

	if err := flagSet.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	self := filepath.Base(os.Args[0])
	if self == "installtool" {
		self = "installtool.go"
	}

	versions := map[string]string{
		"addlicense":    "1.1.1",
		"gci":           "0.13.6",
		"go-licenses":   "1.6.0",
		"gofumpt":       "0.8.0",
		"golangci-lint": "2.1.6",
		"golines":       "0.12.2",
	}
	version, ok := versions[tool]
	if !ok {
		log.Fatalf("Unknown tool: %s", tool)
	}

	if !shouldInstall(tool, version) && !*force {
		fmt.Printf("%s: `%s` is up to date.\n", self, tool)

		return
	}

	switch tool {
	case "addlicense":
		goInstall(*exe, "github.com/google/addlicense", version)
	case "gci":
		goInstall(*exe, "github.com/daixiang0/gci", version)
	case "go-licenses":
		goInstall(*exe, "github.com/google/go-licenses", version)
	case "gofumpt":
		goInstall(*exe, "mvdan.cc/gofumpt", version)
	case "golangci-lint":
		installGolangciLint(*exe, version)
	case "golines":
		installGolines(*exe, version)
	default:
		log.Fatalf("Unknown tool: %s", tool)
	}
}

func goEnv(exe, key string) string {
	cmd := exec.Command(exe, "env", key)

	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("Failed to run go env %s: %v", key, err)
	}

	return strings.TrimSpace(string(out))
}

func goInstall(exe, mod, version string) {
	err := run.Run(exe, "install", mod+"@v"+version)
	if err != nil {
		log.Fatalf("Failed to install %s: %v", mod, err)
	}
}

func installGolangciLint(exe, version string) {
	gopath := goEnv(exe, "GOPATH")
	installDir := filepath.Join(gopath, "bin")
	scriptURL := "https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh"

	resp, err := http.Get(scriptURL)
	if err != nil {
		log.Fatalf("Failed to download golangci-lint install script: %v", err)
	}
	defer resp.Body.Close()

	err = run.Run("sh", "-s", "--", "-b", installDir, "v"+version)
	if err != nil {
		log.Fatalf("Failed to install golangci-lint: %v", err)
	}
}

func installGolines(exe, version string) {
	refEndpoint := "https://api.github.com/repos/segmentio/golines/git/ref/tags/v" + version

	resp, err := http.Get(refEndpoint)
	if err != nil {
		log.Fatalf("Failed to download golines ref info: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read golines ref info: %v", err)
	}

	refJSON := make(map[string]any)

	if err := json.Unmarshal(data, &refJSON); err != nil {
		log.Fatalf("Failed to parse golines ref info: %v", err)
	}

	rawObject, ok := refJSON["object"]
	if !ok {
		log.Fatalf("Failed to get golines ref object")
	}

	object, ok := rawObject.(map[string]any)
	if !ok {
		log.Fatalf("Failed to parse golines ref object")
	}

	rawSHA, ok := object["sha"]
	if !ok {
		log.Fatalf("Failed to get golines ref sha")
	}

	sha, ok := rawSHA.(string)
	if !ok {
		log.Fatalf("Failed to parse golines ref sha")
	}

	err = run.Run(
		exe,
		"install",
		"-ldflags",
		fmt.Sprintf(
			"-X main.version=%s -X main.commit=%s -X main.date=%s",
			version,
			sha,
			time.Now().UTC().Format(time.RFC3339),
		),
		"github.com/segmentio/golines@v"+version,
	)
	if err != nil {
		log.Fatalf("Failed to install github.com/segmentio/golines: %v", err)
	}
}

func shouldInstall(tool, version string) bool {
	if p, err := exec.LookPath(tool); p == "" || err != nil {
		return true
	}

	if tool == "addlicense" || tool == "go-licenses" {
		return false
	}

	out, err := exec.Command(tool, "--version").Output()
	if err != nil {
		log.Fatalf("Failed to check %s version: %v", tool, err)
	}

	var current string

	switch tool {
	case "gci":
		current = strings.Fields(string(out))[2]
	case "gofumpt":
		current = strings.Fields(string(out))[0][1:]
	case "golangci-lint":
		current = strings.Fields(string(out))[3]
	case "golines":
		current = strings.Fields(string(out))[1][1:]
	default:
		log.Fatalf("Unknown tool: %s", tool)
	}

	if current == version {
		return false
	}

	return true
}
