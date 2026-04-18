GO ?= go
PKG ?= ./...
RUN ?= .
COVER_FILE ?= coverage.out
GOCACHE_DIR ?= $(CURDIR)/.gocache
GOMODCACHE_DIR ?= $(CURDIR)/.gomodcache
GOLANGCI_LINT_CACHE_DIR ?= $(CURDIR)/.golangci-lint-cache
GO_ENV ?= GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)"
VERSION ?=
MAIN_BRANCH ?= main

.PHONY: help fmt fmt-check vet lint test test-pkg test-name test-cover test-race bench ci release-tag release-gh release

help:
	@echo "Targets:"
	@echo "  make test                      Run tests for all packages"
	@echo "  make test-pkg PKG=./reqx       Run tests for a specific package"
	@echo "  make test-name PKG=./reqx RUN=TestDecodeJSON"
	@echo "                                 Run a specific test by name"
	@echo "  make test-cover                Run tests with coverage"
	@echo "  make test-race                 Run tests with race detector"
	@echo "  make bench                     Run benchmarks with benchmem"
	@echo "  make vet                       Run go vet"
	@echo "  make lint                      Run golangci-lint"
	@echo "  make fmt-check                 Check gofmt status"
	@echo "  make ci                        Run fmt-check, vet, test, lint"
	@echo "  make release-tag VERSION=vX.Y.Z [MAIN_BRANCH=branch]"
	@echo "                                 Create and push an annotated tag from MAIN_BRANCH"
	@echo "                                 (must be on MAIN_BRANCH and synced with origin)"
	@echo "  make release-gh VERSION=vX.Y.Z Create GitHub release notes for an existing tag"
	@echo "  make release VERSION=vX.Y.Z [MAIN_BRANCH=branch]"
	@echo "                                 Run release-tag and release-gh"

fmt:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) fmt ./...

fmt-check:
	@unformatted="$$(find . -path './.gomodcache' -prune -o -name '*.go' -type f -exec gofmt -l {} +)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) vet ./...

lint:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)" "$(GOLANGCI_LINT_CACHE_DIR)"
	@$(GO_ENV) GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE_DIR)" golangci-lint run ./...

test:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test ./...

test-pkg:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test $(PKG)

test-name:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test $(PKG) -run $(RUN)

test-cover:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test ./... -cover -coverprofile=$(COVER_FILE)
	@$(GO_ENV) $(GO) tool cover -func=$(COVER_FILE)

test-race:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test ./... -race

bench:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@$(GO_ENV) $(GO) test ./... -run '^$$' -bench . -benchmem

ci: fmt-check vet test lint

release-tag:
	@test -n "$(VERSION)" || (echo "Usage: make release-tag VERSION=vX.Y.Z"; exit 1)
	@case "$(VERSION)" in v*) ;; *) echo "VERSION must start with v (for example: v0.2.0)"; exit 1;; esac
	@current_branch="$$(git branch --show-current)"; \
	if [ "$$current_branch" != "$(MAIN_BRANCH)" ]; then \
		echo "release-tag must run on $(MAIN_BRANCH). Current branch: $$current_branch"; \
		exit 1; \
	fi
	@if ! git diff --quiet || ! git diff --cached --quiet; then \
		echo "Working tree is not clean. Commit or stash changes before release."; \
		exit 1; \
	fi
	@git fetch origin "$(MAIN_BRANCH)"
	@local_main="$$(git rev-parse "$(MAIN_BRANCH)")"; \
	remote_main="$$(git rev-parse "origin/$(MAIN_BRANCH)")"; \
	if [ "$$local_main" != "$$remote_main" ]; then \
		echo "$(MAIN_BRANCH) is not up to date with origin/$(MAIN_BRANCH). Run: git pull --ff-only origin $(MAIN_BRANCH)"; \
		exit 1; \
	fi
	@if git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null; then \
		echo "Tag $(VERSION) already exists locally."; \
		exit 1; \
	fi
	@if git ls-remote --tags --exit-code origin "refs/tags/$(VERSION)" >/dev/null 2>&1; then \
		echo "Tag $(VERSION) already exists on origin."; \
		exit 1; \
	fi
	@git tag -a "$(VERSION)" "$(MAIN_BRANCH)" -m "release $(VERSION)"
	@git push origin "$(VERSION)"
	@echo "Created and pushed tag $(VERSION) from $(MAIN_BRANCH)"

release-gh:
	@test -n "$(VERSION)" || (echo "Usage: make release-gh VERSION=vX.Y.Z"; exit 1)
	@if ! git ls-remote --tags --exit-code origin "refs/tags/$(VERSION)" >/dev/null 2>&1; then \
		echo "Tag $(VERSION) does not exist on origin. Run: make release-tag VERSION=$(VERSION) MAIN_BRANCH=$(MAIN_BRANCH)"; \
		exit 1; \
	fi
	@gh release create "$(VERSION)" --generate-notes --verify-tag
	@echo "Created GitHub release $(VERSION)"

release: release-tag release-gh
