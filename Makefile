SHELL := /bin/bash

GOLANGCI_LINT := mise exec -- golangci-lint

.PHONY: fmt
fmt:
	$(GOLANGCI_LINT) fmt

.PHONY: lint
lint:
	$(GOLANGCI_LINT) run
