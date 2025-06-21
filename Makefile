# Copyright 2025 The Reginald Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This Makefile is POSIX-compliant, and non-compliance is considered a bug. It
# follows POSIX.1-2008. Documentation can be found here:
# https://pubs.opengroup.org/onlinepubs/9699919799.2008edition/.

.POSIX:
.SUFFIXES:

GO = go
GOFLAGS =

VERSION =
OUTPUT =

TOOLFLAGS =

ALLOWED_LICENSES = Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT
COPYRIGHT_HOLDER = The Reginald Authors
LICENSE = apache
ADDLICENSE_PATTERNS = *.go internal plugins tools

GO_MODULE = github.com/reginald-project/reginald

RM = rm -f

# Default target.
all: reginald plugins

# CODE QUALITY & CHECKS

audit: license-check test lint
	"$(GO)" mod tidy -diff
	"$(GO)" mod verify

license-check: go-licenses
	"$(GO)" mod verify
	"$(GO)" mod download
	go-licenses check --include_tests $(GO_MODULE)/... --allowed_licenses="$(ALLOWED_LICENSES)"

lint: addlicense golangci-lint
	addlicense -check -c "$(COPYRIGHT_HOLDER)" -l "$(LICENSE)" $(ADDLICENSE_PATTERNS)
	golangci-lint config verify
	golangci-lint run

test: FORCE
	"$(GO)" test $(GOFLAGS) ./...

# DEVELOPMENT & BUILDING

tidy: addlicense gci gofumpt golines
	addlicense -v -c "$(COPYRIGHT_HOLDER)" -l "$(LICENSE)" $(ADDLICENSE_PATTERNS)
	"$(GO)" mod tidy -v
	gci write .
	golines --no-chain-split-dots --no-reformat-tags -w .
	gofumpt -extra -l -w .

reginald: FORCE buildtask
	@./buildtask $@

build: reginald

plugins: reginald-go

reginald-go: FORCE
	mkdir -p ./bin/go
	cp ./plugins/go/manifest.json ./bin/go/manifest.json
	"$(GO)" build -o ./bin/go/reginald-go ./plugins/go

clean: FORCE
	@exe=""; \
	\
	case "$$("$(GO)" env GOOS)" in \
		windows) exe=".exe";; \
	esac; \
	\
	output="$(OUTPUT)"; \
	\
	if [ -z "$${output}" ]; then \
		output="reginald$${exe}"; \
	fi; \
	\
	$(RM) "$${output}"
	@$(RM) -r bin

# TOOL HELPERS

addlicense delve gci go-licenses gofumpt golangci-lint golines: FORCE installtool
	@./installtool $@ $(TOOLFLAGS)

buildtask: tools/buildtask/tool.go
	"$(GO)" build -o $@ -tags tool $<

installtool: tools/installtool/tool.go
	"$(GO)" build -o $@ -tags tool $<

# SPECIAL TARGET

FORCE: ;
