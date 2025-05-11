.DEFAULT_GOAL := build

GCI_VERSION ?= 0.13.0
GOFUMPT_VERSION ?= 0.8.0
GOLANGCI_LINT_VERSION ?= 2.1.6
GOLINES_VERSION ?= 0.12.2

GOPATH = $(shell go env GOPATH)

.PHONY: build
build:
	go build -o reginald ./cmd/reginald

# Linting and formatting

.PHONY: fmt
fmt: install-gci install-gofumpt install-golines
	go mod tidy
	gci write .
	golines --no-chain-split-dots -w .
	gofumpt -extra -l -w .

.PHONY: lint
lint: install-golangci-lint
	golangci-lint run

# Tools

.PHONY: install-gci
install-gci:
ifeq (, $(shell which gci))
	@echo "gci not found, installing..."
	go install github.com/daixiang0/gci@v$(GCI_VERSION)
endif
ifneq ($(GCI_VERSION), $(shell gci --version | awk '{print $$3}'))
	@echo "found gci version $(shell gci --version | awk '{print $$3}'), installing version $(GCI_VERSION)..."
	go install github.com/daixiang0/gci@v$(GCI_VERSION)
endif

.PHONY: install-gofumpt
install-gofumpt:
ifeq (, $(shell which gofumpt))
	@echo "gofumpt not found, installing..."
	go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION)
endif
ifneq ($(GOFUMPT_VERSION), $(shell gofumpt --version | awk '{print $$1}' | cut -c 2-))
	@echo "found gofumpt version $(shell gofumpt --version | awk '{print $$1}' | cut -c 2-), installing version $(GOFUMPT_VERSION)..."
	go install mvdan.cc/gofumpt@v$(GOFUMPT_VERSION)
endif

.PHONY: install-golangci-lint
install-golangci-lint:
ifeq (, $(shell which golangci-lint))
	@echo "golangci-lint not found, installing..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOPATH)/bin v$(GOLANGCI_LINT_VERSION)
endif
ifneq ($(GOLANGCI_LINT_VERSION), $(shell golangci-lint --version | awk '{print $$4}'))
	@echo "found golangci-lint version $(shell golangci-lint --version | awk '{print $$4}'), installing version $(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOPATH)/bin v$(GOLANGCI_LINT_VERSION)
endif

.PHONY: install-golines
install-golines:
ifeq (, $(shell which golines))
	@echo "golines not found, installing..."
	./scripts/install_golines "$(GOLINES_VERSION)"
endif
ifneq ($(GOLINES_VERSION), $(shell golines --version | head -1 | awk '{print $$2}' | cut -c 2-))
	@echo "found golines version $(shell golines --version | head -1 | awk '{print $$2}' | cut -c 2-), installing version $(GOLINES_VERSION)..."
	./scripts/install_golines "$(GOLINES_VERSION)"
endif
