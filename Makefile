SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

ifndef VERBOSE
# --silent drops the need to prepend `@` to suppress command output.
MAKEFLAGS += --silent
endif

## Build the host gbx binary into bin/
.PHONY: build
build:
	mkdir -p bin
	go build -o bin/gbx ./cmd/gbx

## Regenerate share/man/man1/gbx.1 from docs/gbx.1.md
.PHONY: man
man:
	command -v go-md2man >/dev/null 2>&1 || { \
	  echo 'go-md2man not on PATH. Install with: go install github.com/cpuguy83/go-md2man/v2@latest' >&2; \
	  exit 1; \
	}
	mkdir -p share/man/man1
	go-md2man -in docs/gbx.1.md -out share/man/man1/gbx.1

# Run the pure-bash test suite. Optionally filter to one or more files:
#   make test FILE=tests/41-wrapper-cd.sh
#   make test FILE="tests/41-wrapper-cd.sh tests/43-wrapper-run.sh"
## Run the pure-bash test suite
.PHONY: test
test:
	./scripts/run-tests.sh $(FILE)

## Run the Go test suite at the repo root
.PHONY: test-go
test-go:
	go test ./...

## Run both the Go and bash test suites (pre-push gate)
.PHONY: test-all
test-all: test-go test

## Run integration tests in parallel (WORKERS=4 by default)
.PHONY: test-parallel
test-parallel:
	./scripts/parallel-tests.sh

## Install golangci-lint into ./bin at the pinned version
.PHONY: install-golangci-lint
install-golangci-lint:
	./scripts/install-golangci-lint.sh

## Run gofmt check, go vet, and golangci-lint
.PHONY: lint
lint: install-golangci-lint
	echo '==> gofmt'
	out="$$(gofmt -l . | grep -v '^docs/superpowers/' || true)"; \
	if [ -n "$$out" ]; then \
	  printf 'gofmt would reformat:\n%s\n' "$$out" >&2; exit 1; \
	fi
	echo '==> go vet'
	go vet ./...
	echo '==> golangci-lint'
	./bin/golangci-lint run

## Remove build artifacts and stale .test-config* directories
.PHONY: clean
clean:
	rm -rf bin/gbx bin/golangci-lint share/man/man1/gbx.1 .test-config .test-config.w*

# Wipe Docker residue left by the bash test suite. Five passes, in order:
#   1. Containers labeled io.glovebox.test=1 (every test-mode `gbx` creates
#      with this label - precise, future-proof signal).
#   2. Legacy workspace-orphan agents (any glovebox-agent-* whose /workspace
#      bind source is under TMPDIR). Covers containers created before the
#      label scheme landed.
#   3. Test-only agent images (glovebox-agent-test-*) left by the AAD
#      rebuild test's throwaway GBX_AGENT_IMAGE.
#   4. glovebox-stack-* networks with zero containers attached.
#   5. Dangling volumes via `docker volume prune -f` (only removes volumes
#      no container references - safe by definition).
# Plus `.test-config*` dirs so a fresh suite run starts clean.
# Never touches the operator's real agents, the singleton stack, or the
# `glovebox-agent:local` image.
## Wipe Docker test residue (labeled containers, TMPDIR agents, dangling stack nets/vols)
.PHONY: clean-tests
clean-tests:
	./scripts/clean-tests.sh

# Tag a release. By default re-tags the current version.txt without bumping
# (so version.txt edited by hand can be tagged directly). Pass BUMP=patch,
# BUMP=minor, or BUMP=major to bump first, commit the bump, then tag.
# The push and Formula update are intentionally left manual - the Homebrew
# tarball sha256 only exists after the tag is on GitHub.
## Tag a release at current version.txt (default) or BUMP=patch|minor|major
.PHONY: release
release:
	./scripts/release.sh

# Hard reset: removes every glovebox container, network, volume, the agent
# image, and the user's config dir (${GBX_CONFIG_DIR:-~/.config/glovebox}).
# Prompts unless FORCE=1.
## Remove ALL glovebox state (containers, image, config); FORCE=1 skips prompt
.PHONY: uninstall
uninstall:
	./scripts/uninstall.sh

## Create this help message
.PHONY: help
help:
	awk 'BEGIN { printf "\nUsage:\n  make \033[36m<target>\033[0m\n\n" } \
		/^## / { desc = substr($$0, 4); next } \
		/^[a-zA-Z0-9][a-zA-Z0-9_-]*:/ { \
		if (desc) { name = $$1; sub(/:.*/, "", name); \
			printf "  \033[36m%-27s\033[0m %s\n", name, desc; desc = "" } \
		} \
		END { printf "\n" }' $(MAKEFILE_LIST)
