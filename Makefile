# Copyright 2025 Antti Kivi
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
# follows the POSIX base specification IEEE Std 1003.1-2024. Documentation can
# be found here: https://pubs.opengroup.org/onlinepubs/9799919799/.

.POSIX:
.SUFFIXES:

GO = go
GOFLAGS ?=

ADDLICENSE_VERSION = 1.1.1
GCI_VERSION = 0.13.6
GO_LICENSES_VERSION = 1.6.0
GOFUMPT_VERSION = 0.8.0
GOLANGCI_LINT_VERSION = 2.1.6
GOLINES_VERSION = 0.12.2

ALLOWED_LICENSES = Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT

COPYRIGHT_HOLDER = Antti Kivi
LICENSE = apache
ADDLICENSE_PATTERNS = *.go examples internal pkg scripts

GO_MODULE = github.com/anttikivi/reginald
VERSION_PACKAGE = github.com/anttikivi/reginald/pkg/version

# Default target.
.PHONY: all
all: build plugins

# CODE QUALITY & CHECKS

.PHONY: audit
audit: license-check test lint
	$(GO) mod tidy -diff
	$(GO) mod verify

.PHONY: license-check
license-check: go-licenses
	$(GO) mod verify
	$(GO) mod download
	go-licenses check --include_tests $(GO_MODULE)/... --allowed_licenses="$(ALLOWED_LICENSES)"

.PHONY: lint
lint: addlicense golangci-lint
	addlicense -check -c "$(COPYRIGHT_HOLDER)" -l "$(LICENSE)" $(ADDLICENSE_PATTERNS)
	golangci-lint run

.PHONY: test
test: go
	$(GO) test $(GOFLAGS) ./...

# DEVELOPMENT & BUILDING

.PHONY: tidy
tidy: addlicense gci go gofumpt golines
	addlicense -v -c "$(COPYRIGHT_HOLDER)" -l "$(LICENSE)" $(ADDLICENSE_PATTERNS)
	$(GO) mod tidy -v
	gci write .
	golines --no-chain-split-dots -w .
	gofumpt -extra -l -w .

.PHONY: fmt
fmt: tidy

.PHONY: build
build: go
	@base_version="$$(cat VERSION)"; \
	prerelease="$(PRERELEASE)"; \
	\
	if [ -z "$${prerelease}" ]; then \
		prerelease="0.dev.$$(date -u +"%Y%m%d%H%M%S")"; \
	fi; \
	\
	if [ -n "$(VERSION)" ]; then \
		version="$(VERSION)"; \
	else \
		version="$${base_version}"; \
		if [ -n "$${prerelease}" ]; then \
			version="$${version}-$${prerelease}"; \
		fi; \
		if [ -n "$(BUILD_METADATA)" ]; then \
			version="$${version}+$(BUILD_METADATA)"; \
		fi; \
	fi; \
	\
	exe=""; \
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
	set -- \
		-o "$${output}" \
		$(GOFLAGS) \
		-ldflags "-X $(VERSION_PACKAGE).buildVersion=$${version}" \
	; \
	\
	printf 'building Reginald version %s\n' "$${version}"; \
	GOFLAGS= $(GO) build "$$@"

.PHONY: plugins
plugins: example-plugin

.PHONY: example-plugin
example-plugin: go
	$(GO) build -o reginald-example ./examples

.PHONY: clean
clean:
	@exe=""; \
	\
	case "$$($(GO) env GOOS)" in \
		windows) exe=".exe";; \
	esac; \
	\
	output="$(OUTPUT)"; \
	\
	if [ -z "$${output}" ]; then \
		output="reginald$${exe}"; \
	fi; \
	\
	rm "$${output}"
	@rm reginald-example

# TOOL HELPERS

.PHONY: addlicense
addlicense:
	@./scripts/install_tool "$(GO)" "$@" "$(ADDLICENSE_VERSION)" "$(FORCE_REINSTALL)"

.PHONY: gci
gci:
	@./scripts/install_tool "$(GO)" "$@" "$(GCI_VERSION)" "$(FORCE_REINSTALL)"

.PHONY: go
go:
	@if ! command -v "$(GO)" >/dev/null 2>&1; then \
		printf 'Error: the Go executable was not found, tried "%s"\n' "$(GO)" >&2; \
		exit 1; \
	else \
		printf 'using Go version %s\n' "$$("$(GO)" version | awk '{print $$3}' | cut -c 3-)"; \
	fi

.PHONY: go-licenses
go-licenses:
	@./scripts/install_tool "$(GO)" "$@" "$(GO_LICENSES_VERSION)" "$(FORCE_REINSTALL)"

.PHONY: gofumpt
gofumpt:
	@./scripts/install_tool "$(GO)" "$@" "$(GOFUMPT_VERSION)" "$(FORCE_REINSTALL)"

.PHONY: golangci-lint
golangci-lint:
	@./scripts/install_tool "$(GO)" "$@" "$(GOLANGCI_LINT_VERSION)" "$(FORCE_REINSTALL)"

.PHONY: golines
golines:
	@./scripts/install_tool "$(GO)" "$@" "$(GOLINES_VERSION)" "$(FORCE_REINSTALL)"
