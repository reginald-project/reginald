.POSIX:
.SUFFIXES:

GCI_VERSION = 0.13.0
GOFUMPT_VERSION = 0.8.0
GOLANGCI_LINT_VERSION = 2.1.6
GOLINES_VERSION = 0.12.2

all: build

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

# Linting and formatting

fmt: install-gci install-gofumpt install-golines
	go mod tidy
	gci write .
	golines --no-chain-split-dots -w .
	gofumpt -extra -l -w .

lint: install-golangci-lint
	golangci-lint run

# Tools

install-gci:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v gci >/dev/null 2>&1; then \
		echo "gci not found, installing..."; \
		go install github.com/daixiang0/gci@v$(GCI_VERSION); \
		exit 0; \
	fi; \
	CURRENT_VERSION="$$(gci --version 2>/dev/null | awk '{print $$3}')"; \
	if [ "$${CURRENT_VERSION}" != "$(GCI_VERSION)" ]; then \
		echo "found gci version $${CURRENT_VERSION}, installing version $(GCI_VERSION)..."; \
		go install github.com/daixiang0/gci@v$(GCI_VERSION); \
	fi

install-gofumpt:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v gofumpt >/dev/null 2>&1; then \
		echo "gofumpt not found, installing..."; \
		go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION); \
		exit 0; \
	fi; \
	CURRENT_VERSION="$$(gofumpt --version | awk '{print $$1}' | cut -c 2-)"; \
	if [ "$${CURRENT_VERSION}" != "$(GOFUMPT_VERSION)" ]; then \
		echo "found gofumpt version $${CURRENT_VERSION}, installing version $(GOFUMPT_VERSION)..."; \
		go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION); \
	fi

install-golangci-lint:
	@GOPATH="$$(go env GOPATH)"; \
	PATH="$${PATH}:$${GOPATH}/bin"; \
	if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found, installing..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "${GOPATH}/bin" v$(GOLANGCI_LINT_VERSION); \
		exit 0; \
	fi; \
	CURRENT_VERSION=$$(golangci-lint --version 2>/dev/null | awk '{print $$4}'); \
	if [ "$${CURRENT_VERSION}" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "found golangci-lint version $${CURRENT_VERSION}, installing version $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "${GOPATH}/bin" v$(GOLANGCI_LINT_VERSION); \
	fi

install-golines:
	@PATH="$${PATH}:$$(go env GOPATH)/bin"; \
	if ! command -v golines >/dev/null 2>&1; then \
		echo "golines not found, installing..."; \
		./scripts/install_golines "$(GOLINES_VERSION)"; \
		exit 0; \
	fi; \
	CURRENT_VERSION="$$(golines --version | head -1 | awk '{print $$2}' | cut -c 2-)"; \
	if [ "$${CURRENT_VERSION}" != "$(GOLINES_VERSION)" ]; then \
		echo "found golines version $${CURRENT_VERSION}, installing version $(GOLINES_VERSION)..."; \
		./scripts/install_golines "$(GOLINES_VERSION)"; \
	fi
