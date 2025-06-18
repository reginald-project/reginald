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
)

func main() {
	log.SetFlags(0)

	flagSet := flag.NewFlagSet("installtool", flag.ExitOnError)
	exe := flagSet.String("go", "go", "path to the Go executable")
	tool := flagSet.String("t", "", "name of the tool to install")
	force := flagSet.Bool("f", false, "reinstall the tool if it is already installed")
	flagSet.Usage = func() {
		fmt.Fprintln(flagSet.Output(), "Usage: installtool [flags]")
		flagSet.PrintDefaults()
	}

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	if *tool == "" {
		flagSet.Usage()
		os.Exit(2)
	}

	versions := map[string]string{
		"addlicense":    "1.1.1",
		"gci":           "0.13.6",
		"go-licenses":   "1.6.0",
		"gofumpt":       "0.8.0",
		"golangci-lint": "2.1.6",
		"golines":       "0.12.2",
	}
	version, ok := versions[*tool]
	if !ok {
		log.Fatalf("Unknown tool: %s", *tool)
	}

	if !shouldInstall(*tool, version) && !*force {
		log.Printf("%s is already installed", *tool)

		return
	}

	log.Printf("Installing %s version %s", *tool, version)

	switch *tool {
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
		log.Fatalf("Unknown tool: %s", *tool)
	}

	log.Printf("Installed %s version %s", *tool, version)
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
	cmd := exec.Command(exe, "install", mod+"@v"+version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
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

	cmd := exec.Command("sh", "-s", "--", "-b", installDir, "v"+version)
	cmd.Stdin = resp.Body
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
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

	cmd := exec.Command(
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
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
		log.Printf("Found %s version %s", tool, current)

		return false
	}

	log.Printf("Found %s version %s, want %s", tool, current, version)

	return true
}
