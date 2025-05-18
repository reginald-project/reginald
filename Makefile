# This Makefile is POSIX-compliant, and non-compliance is considered a bug. It
# follows the POSIX base specification IEEE Std 1003.1-2024. The specification
# can be found here: https://pubs.opengroup.org/onlinepubs/9799919799/.

.POSIX:
.SUFFIXES:

GCI_VERSION = 0.13.0
GOFUMPT_VERSION = 0.8.0
GOLANGCI_LINT_VERSION = 2.1.6
GOLINES_VERSION = 0.12.2

# Default target.
.PHONY: all
all: build plugins

# ============================================================================ #
# QUALITY CONTROL
# ============================================================================ #

.PHONY: audit
audit: test lint
	go mod tidy -diff
	go mod verify

.PHONY: lint
lint: install-golangci-lint
	golangci-lint run

.PHONY: test
test:
	go test $(GOFLAGS) ./...

# ============================================================================ #
# DEVELOPMENT & BUILDING
# ============================================================================ #

.PHONY: tidy
tidy: install-gci install-gofumpt install-golines
	go mod tidy -v
	gci write .
	golines --no-chain-split-dots -w .
	gofumpt -extra -l -w .

.PHONY: build
build:
	@commit_hash="$$(git describe --always --dirty --abbrev=40)"; \
	build_date="$$(date -u +"%Y-%m-%dT%H:%M:%SZ")"; \
	base_version="$$(cat VERSION)"; \
	prerelease="$(PRERELEASE)"; \
	build_metadata="$(BUILD_METADATA)"; \
	\
	if [ -z "$${prerelease}" ]; then \
		prerelease="0.dev.$$(date -u +"%Y%m%d%H%M%S")"; \
	fi; \
	\
	if [ -z "$${build_metadata}" ]; then \
		build_metadata="$${commit_hash}"; \
	fi; \
	\
	if [ -n "$(VERSION)" ]; then \
		version="$(VERSION)"; \
	else \
		version="$${base_version}"; \
		if [ -n "$${prerelease}" ]; then \
			version="$${version}-$${prerelease}"; \
		fi; \
		if [ -n "$${build_metadata}" ]; then \
			version="$${version}+$${build_metadata}"; \
		fi; \
	fi; \
	\
	ldflags="$(LDFLAGS)"; \
	ldflags="$${ldflags} -X github.com/anttikivi/reginald/internal/version.Version=$${version}"; \
	ldflags="$${ldflags} -X github.com/anttikivi/reginald/internal/version.Commit=$${commit_hash}"; \
	ldflags="$${ldflags} -X github.com/anttikivi/reginald/internal/version.BuildDate=$${build_date}"; \
	\
	goflags="$(GOFLAGS)"; \
	\
	exe=""; \
	\
	case "$$(go env GOOS)" in \
		windows) exe=".exe";; \
	esac; \
	\
	output="$(OUTPUT)"; \
	\
	if [ -z "$${output}" ]; then \
		output="reginald$${exe}"; \
	fi; \
	\
	echo "building Reginald version $${version}"; \
	go build $${goflags} -ldflags "$${ldflags}" -o "$${output}" ./cmd/reginald

.PHONY: plugins
plugins: theme

.PHONY: theme
theme:
	go build -o reginald-theme ./plugins/theme

.PHONY: clean
clean:
	@exe=""; \
	\
	case "$$(go env GOOS)" in \
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
	@rm reginald-theme

# ============================================================================ #
# TOOL HELPERS
# ============================================================================ #

.PHONY: install-gci
install-gci:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v gci >/dev/null 2>&1; then \
		echo "gci not found, installing..."; \
		go install github.com/daixiang0/gci@v$(GCI_VERSION); \
		exit 0; \
	fi; \
	current_version="$$(gci --version 2>/dev/null | awk '{print $$3}')"; \
	if [ "$${current_version}" != "$(GCI_VERSION)" ]; then \
		echo "found gci version $${current_version}, installing version $(GCI_VERSION)..."; \
		go install github.com/daixiang0/gci@v$(GCI_VERSION); \
	fi

.PHONY: install-gofumpt
install-gofumpt:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v gofumpt >/dev/null 2>&1; then \
		echo "gofumpt not found, installing..."; \
		go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION); \
		exit 0; \
	fi; \
	current_version="$$(gofumpt --version | awk '{print $$1}' | cut -c 2-)"; \
	if [ "$${current_version}" != "$(GOFUMPT_VERSION)" ]; then \
		echo "found gofumpt version $${current_version}, installing version $(GOFUMPT_VERSION)..."; \
		go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION); \
	fi

.PHONY: install-golangci-lint
install-golangci-lint:
	@GOPATH="$$(go env GOPATH)"; \
	PATH="$${PATH}:$${GOPATH}/bin"; \
	if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found, installing..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "${GOPATH}/bin" v$(GOLANGCI_LINT_VERSION); \
		exit 0; \
	fi; \
	current_version=$$(golangci-lint --version 2>/dev/null | awk '{print $$4}'); \
	if [ "$${current_version}" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "found golangci-lint version $${current_version}, installing version $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "${GOPATH}/bin" v$(GOLANGCI_LINT_VERSION); \
	fi

.PHONY: install-golines
install-golines:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v golines >/dev/null 2>&1; then \
		echo "golines not found, installing..."; \
		./scripts/install_golines "$(GOLINES_VERSION)"; \
		exit 0; \
	fi; \
	current_version="$$(golines --version | head -1 | awk '{print $$2}' | cut -c 2-)"; \
	if [ "$${current_version}" != "$(GOLINES_VERSION)" ]; then \
		echo "found golines version $${current_version}, installing version $(GOLINES_VERSION)..."; \
		./scripts/install_golines "$(GOLINES_VERSION)"; \
	fi
