SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the host gbx binary into bin/
	@mkdir -p bin
	go build -o bin/gbx ./cmd/gbx

.PHONY: man
man: ## Regenerate share/man/man1/gbx.1 from docs/gbx.1.md
	@command -v go-md2man >/dev/null 2>&1 || { \
	  echo 'go-md2man not on PATH. Install with: go install github.com/cpuguy83/go-md2man/v2@latest' >&2; \
	  exit 1; \
	}
	@mkdir -p share/man/man1
	go-md2man -in docs/gbx.1.md -out share/man/man1/gbx.1

.PHONY: test
# Run the pure-bash test suite. Optionally filter to one or more files:
#   make test FILE=tests/41-wrapper-cd.sh
#   make test FILE="tests/41-wrapper-cd.sh tests/43-wrapper-run.sh"
test: ## Run the pure-bash test suite
	@./scripts/run-tests.sh $(FILE)

.PHONY: test-go
test-go: ## Run the Go test suite at the repo root
	go test ./...

.PHONY: test-all
test-all: test-go test ## Run both the Go and bash test suites (pre-push gate)

.PHONY: test-parallel
test-parallel: ## Run integration tests in parallel (WORKERS=4 by default)
	@./scripts/parallel-tests.sh

.PHONY: install-golangci-lint
install-golangci-lint: ## Install golangci-lint into ./bin at the pinned version
	@./scripts/install-golangci-lint.sh

.PHONY: lint
lint: install-golangci-lint ## Run gofmt check, go vet, and golangci-lint
	@echo '==> gofmt'
	@out="$$(gofmt -l . | grep -v '^docs/superpowers/' || true)"; \
	if [ -n "$$out" ]; then \
	  printf 'gofmt would reformat:\n%s\n' "$$out" >&2; exit 1; \
	fi
	@echo '==> go vet'
	@go vet ./...
	@echo '==> golangci-lint'
	@./bin/golangci-lint run

.PHONY: clean
clean: ## Remove build artifacts and stale .test-config* directories
	@rm -rf bin/gbx bin/golangci-lint share/man/man1/gbx.1 .test-config .test-config.w*

.PHONY: clean-tests
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
clean-tests: ## Wipe Docker test residue (labeled containers, TMPDIR agents, dangling stack nets/vols)
	@command -v docker >/dev/null 2>&1 || { echo 'docker not on PATH; nothing to clean'; exit 0; }
	@echo '==> labeled test containers (io.glovebox.test=1)'
	@docker ps -aq --filter label=io.glovebox.test=1 | xargs -r docker rm -f >/dev/null 2>&1 || true
	@echo '==> legacy workspace-orphan agents (workspace under TMPDIR)'
	@tmp_root="$$(cd "$${TMPDIR:-/tmp}" 2>/dev/null && pwd -P)"; \
	for c in $$(docker ps -a --filter name=glovebox-agent- --format '{{.Names}}' 2>/dev/null); do \
	  ws="$$(docker inspect "$$c" --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}' 2>/dev/null)"; \
	  case "$$ws" in "$$tmp_root"/*|/tmp/*|/private/tmp/*) docker rm -f "$$c" >/dev/null 2>&1 || true ;; esac; \
	done
	@echo '==> test-only agent images (glovebox-agent-test-*)'
	@docker images --filter reference='glovebox-agent-test-*' -q 2>/dev/null | xargs -r docker rmi -f >/dev/null 2>&1 || true
	@echo '==> dangling images (orphan layers from previous rebuilds)'
	@docker image prune -f >/dev/null 2>&1 || true
	@echo '==> empty glovebox-stack-* networks'
	@for n in $$(docker network ls --format '{{.Name}}' 2>/dev/null | grep '^glovebox-stack-' || true); do \
	  count="$$(docker network inspect "$$n" --format '{{len .Containers}}' 2>/dev/null)"; \
	  if [ "$$count" = "0" ]; then docker network rm "$$n" >/dev/null 2>&1 || true; fi; \
	done
	@echo '==> dangling volumes'
	@docker volume prune -f >/dev/null 2>&1 || true
	@echo '==> .test-config* dirs'
	@rm -rf .test-config .test-config.w*
	@echo 'Test residue cleaned.'

.PHONY: release
# Tag a release. By default re-tags the current version.txt without bumping
# (so version.txt edited by hand can be tagged directly). Pass BUMP=patch,
# BUMP=minor, or BUMP=major to bump first, commit the bump, then tag.
# The push and Formula update are intentionally left manual - the Homebrew
# tarball sha256 only exists after the tag is on GitHub.
release: ## Tag a release at current version.txt (default) or BUMP=patch|minor|major
	@set -e; \
	bump="$${BUMP:-}"; \
	if [ -n "$$(git status --porcelain)" ]; then \
	  echo "Working tree not clean; commit or stash first." >&2; exit 1; \
	fi; \
	current="$$(cat version.txt | tr -d '[:space:]')"; \
	if [ -z "$$bump" ]; then \
	  new="$$current"; \
	  echo "Tagging current version v$$new (no bump)"; \
	else \
	  IFS=. read -r major minor patch <<< "$$current"; \
	  case "$$bump" in \
	    major) new="$$((major + 1)).0.0" ;; \
	    minor) new="$$major.$$((minor + 1)).0" ;; \
	    patch) new="$$major.$$minor.$$((patch + 1))" ;; \
	    *) echo "BUMP must be one of: patch, minor, major (or unset to re-tag current)" >&2; exit 1 ;; \
	  esac; \
	  echo "$$current → $$new ($$bump bump)"; \
	  echo "$$new" > version.txt; \
	  git add version.txt; \
	  git commit -m "chore: release v$$new"; \
	fi; \
	if git rev-parse --verify "v$$new" >/dev/null 2>&1; then \
	  echo "Tag v$$new already exists locally; pick a new version with BUMP=…" >&2; exit 1; \
	fi; \
	git tag -a "v$$new" -m "Release v$$new"; \
	echo; \
	echo "Created tag v$$new locally. To publish:"; \
	echo "  1) git push && git push origin v$$new"; \
	echo "  2) curl -sL https://github.com/okulik/glovebox/archive/refs/tags/v$$new.tar.gz | shasum -a 256"; \
	echo "  3) In Formula/glovebox.rb uncomment url/sha256/version and fill them in for v$$new"; \
	echo "  4) git add Formula/glovebox.rb && git commit -m 'brew: pin v$$new' && git push"

.PHONY: uninstall
# Hard reset: removes every glovebox container, network, volume, the agent
# image, and the user's config dir (${GBX_CONFIG_DIR:-~/.config/glovebox}).
# Prompts unless FORCE=1.
uninstall: ## Remove ALL glovebox state (containers, image, config); FORCE=1 skips prompt
	@if [ "$(FORCE)" != "1" ]; then \
	  printf 'This removes every glovebox container, the agent image, and %s. Continue? [y/N] ' "$${GBX_CONFIG_DIR:-$$HOME/.config/glovebox}"; \
	  read ans; [ "$$ans" = "y" ] || { echo 'Aborted.'; exit 1; }; \
	fi
	@docker container ls -a --filter name='^glovebox-' --format '{{.Names}}' | xargs -r docker rm -f >/dev/null 2>&1 || true
	@docker network  ls    --format '{{.Name}}'  | grep '^glovebox'        | xargs -r -I {} docker network rm {} >/dev/null 2>&1 || true
	@docker volume   ls    --format '{{.Name}}'  | grep '^glovebox'        | xargs -r docker volume rm >/dev/null 2>&1 || true
	@docker image    rm glovebox-agent:local >/dev/null 2>&1 || true
	@rm -rf "$${GBX_CONFIG_DIR:-$$HOME/.config/glovebox}"
	@echo "Glovebox uninstalled."
