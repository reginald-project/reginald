.DEFAULT_GOAL := build

GOLANGCI_LINT_VERSION ?= 2.1.6

GOPATH = $(shell go env GOPATH)

.PHONY: build
build:
	go build -o reginald ./cmd/reginald

# Linting

.PHONY: lint
lint: install-golangci-lint
	golangci-lint run

# Tools

# There is probably a better way to do this...
.PHONY: install-golangci-lint
install-golangci-lint:
ifeq (, $(shell which golangci-lint))
	@echo "golangci-lint not found, installing..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOPATH)/bin v$(GOLANGCI_LINT_VERSION)
else
ifneq ($(GOLANGCI_LINT_VERSION), $(shell golangci-lint --version | awk '{print $$4}'))
	@echo "found version $(shell golangci-lint --version | awk '{print $$4}') of golangci-lint, installing version $(GOLANGCI_LINT_VERSION)"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOPATH)/bin v$(GOLANGCI_LINT_VERSION)
endif
endif
