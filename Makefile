SHELL := /usr/bin/env bash
.DEFAULT_GOAL := all

MAKEFLAGS += --no-print-directory

PROJECT_ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY: help # Print this help message.
help:
	@grep -E '^\.PHONY: [a-zA-Z0-9_-]+ .*?# .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = "(: |#)"}; {printf "%-30s %s\n", $$2, $$3}'

.PHONY: all # Build the application.
all: build

.PHONY: build # Build the binary for the current platform.
build:
	./tools/goreleaser.sh build --snapshot --clean --single-target

.PHONY: build-all # Build binaries for all platforms.
build-all:
	./tools/goreleaser.sh build --snapshot --clean

.PHONY: release-snapshot # Create a full release snapshot (binaries, archives, packages).
release-snapshot:
	./tools/goreleaser.sh release --snapshot --clean --skip=publish

.PHONY: install # Build and install the binary to $GOPATH/bin.
install: build
	cp ./dist/admiral_$(shell go env GOOS)_$(shell go env GOARCH)*/admiral $(shell go env GOPATH)/bin/admiral

.PHONY: run # Run the application locally.
run: build
	./dist/admiral_$(shell go env GOOS)_$(shell go env GOARCH)*/admiral

.PHONY: test # Run unit tests.
test:
	go test -race -covermode=atomic ./...

.PHONY: test-verbose # Run unit tests with verbose output.
test-verbose:
	go test -v -race -covermode=atomic ./...

.PHONY: lint # Lint the code.
lint:
	./tools/golangci-lint.sh run --timeout 2m30s

.PHONY: lint-fix # Lint and fix the code.
lint-fix:
	./tools/golangci-lint.sh run --fix
	go mod tidy

.PHONY: fmt # Format the code.
fmt:
	go fmt ./...

.PHONY: verify # Verify go modules are tidy.
verify:
	go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum is not tidy" && exit 1)

.PHONY: fetch # Fetch master and tags from origin.
fetch:
	@git fetch --quiet --tags origin master

.PHONY: check-master # Ensure HEAD is master and matches origin/master.
check-master: fetch
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != "master" ]; then \
		echo "Releases are cut from master, not $$(git rev-parse --abbrev-ref HEAD)." >&2; \
		exit 1; \
	fi
	@if [ "$$(git rev-parse HEAD)" != "$$(git rev-parse origin/master)" ]; then \
		echo "Local master does not match origin/master. Pull (or push) before tagging." >&2; \
		exit 1; \
	fi

.PHONY: release # Tag and push the next version (auto-detected from commits).
release: check-master
	@VERSION=$$(./tools/svu.sh next) && \
	echo "Current version: $$(./tools/svu.sh current)" && \
	echo "Next version:    $$VERSION" && \
	echo "" && \
	read -p "Proceed? [y/N] " confirm && [ "$$confirm" = "y" ] && \
	$(MAKE) tag VERSION=$$VERSION

.PHONY: release-patch # Tag and push a patch release.
release-patch: check-master
	@$(MAKE) tag VERSION=$$(./tools/svu.sh patch)

.PHONY: release-minor # Tag and push a minor release.
release-minor: check-master
	@$(MAKE) tag VERSION=$$(./tools/svu.sh minor)

.PHONY: release-major # Tag and push a major release.
release-major: check-master
	@$(MAKE) tag VERSION=$$(./tools/svu.sh major)

.PHONY: tag # Tag and push an explicit version, e.g. make tag VERSION=v1.2.3
tag: check-master
	@if [ -z "$(VERSION)" ]; then \
		echo "VERSION is required. Usage: make tag VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "VERSION must be vX.Y.Z, got $(VERSION)." >&2; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain --untracked-files=no)" ]; then \
		echo "Working tree is dirty. Commit or stash before tagging." >&2; \
		exit 1; \
	fi
	@if git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null; then \
		echo "Tag $(VERSION) already exists." >&2; \
		exit 1; \
	fi
	@echo "Current version: $$(./tools/svu.sh current)"
	@echo "Tagging:         $(VERSION)"
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

.PHONY: version # Show current and next version.
version: fetch
	@echo "Current: $$(./tools/svu.sh current)"
	@echo "Next:    $$(./tools/svu.sh next)"

.PHONY: deps # Download dependencies.
deps:
	go mod download
	go mod tidy
