.DEFAULT_GOAL := help
.PHONY: help dev debug build install test lint vuln license-check publish-check bench cover keys deps-outdated setup prepare-release release release-all release-archive clean ui-test ui-demo

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BIN     := lazyaws
FLAGS   ?=
BUILD_FLAGS := -trimpath
GO_LICENSES_VERSION := v2.0.1
TARGET_GOOS := $(shell go env GOOS)
TARGET_GOARCH := $(shell go env GOARCH)
EXE_SUFFIX := $(if $(filter windows,$(TARGET_GOOS)),.exe,)
DIST_DIR := dist
RELEASE_NAME := $(BIN)-$(VERSION)-$(TARGET_GOOS)-$(TARGET_GOARCH)

help:
	@printf "lazyaws $(VERSION)\nUsage: make <target> [FLAGS=\"-region eu-west-1\"]\n\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

dev: ## Run the TUI from source (extra args via FLAGS=...)
	go run $(LDFLAGS) . $(FLAGS)

debug: ## Run the TUI logging to ~/.lazyaws/debug.log (tail it in another shell)
	go run $(LDFLAGS) . -debug $(FLAGS)

build: ## Build the binary to ./lazyaws with the version stamped in
	go build $(BUILD_FLAGS) $(LDFLAGS) -o $(BIN) .

install: ## Install lazyaws into GOBIN (or GOPATH/bin) so it runs anywhere
	go install $(BUILD_FLAGS) $(LDFLAGS) .

test: ## Run the test suite
	go test ./...

ui-test: ## Drive the built TUI in ttyd against a seeded fake AWS with Playwright (needs docker, ttyd, aws, bun; one-off: cd test/ui && bun install && bunx playwright install chromium)
	bash test/ui/run.sh $(JOURNEYS)

ui-demo: ## Re-record docs/demo.gif against the seeded fake AWS, so the published media carries fabricated data only (needs ui-test's tools plus ffmpeg)
	DRIVER=demo.mjs bash test/ui/run.sh
	ffmpeg -y -f concat -i test/ui/.demo-frames/frames.txt -vf "scale=1010:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5" -loop 0 docs/demo.gif

lint: ## Vet the code and fail on unformatted files
	go vet ./...
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "$$out"; echo "gofmt: files need formatting"; exit 1; fi

vuln: ## Fail on known vulnerabilities reachable from this code
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...

license-check: ## Verify required project and third-party license notices
	@test -s LICENSE
	@test -s ACKNOWLEDGMENTS.md
	@test -s LICENSES/lazydocker-MIT.txt
	@test -s LICENSES/fzf-MIT.txt
	@test -s LICENSES/gocui-BSD-3-Clause.txt
	@grep -q '2018 Jesse Duffield' LICENSES/lazydocker-MIT.txt
	@grep -q '2013-2026 Junegunn Choi' LICENSES/fzf-MIT.txt
	@grep -q '2014 The gocui Authors' LICENSES/gocui-BSD-3-Clause.txt
	@grep -q 'LICENSES/lazydocker-MIT.txt' ACKNOWLEDGMENTS.md
	@grep -q 'LICENSES/fzf-MIT.txt' ACKNOWLEDGMENTS.md
	@grep -q 'LICENSES/gocui-BSD-3-Clause.txt' ACKNOWLEDGMENTS.md
	@grep -q 'ACKNOWLEDGMENTS.md' README.md
	go run github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION) check --include_tests ./...

publish-check: ## Refuse ignored tracked files or source files missing from the index
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		out="$$(git ls-files -ci --exclude-per-directory=.gitignore)"; \
		if [ -n "$$out" ]; then echo "tracked files excluded by .gitignore:"; echo "$$out"; exit 1; fi; \
		missing="$$(for source_file in main.go $$(find apps config ui -type f -name '*.go' -print); do git ls-files --error-unmatch -- "$$source_file" >/dev/null 2>&1 || echo "$$source_file"; done)"; \
		if [ -n "$$missing" ]; then echo "source files missing from the Git index:"; echo "$$missing"; exit 1; fi; \
	fi

bench: ## Run the hot-path benchmarks (list rerender, fit table, overview formatters, command bar, fuzzy ranking, chat render)
	go test ./ui/ ./ui/utils/ ./ui/presentation/ ./ui/resources/ ./ui/fuzzy/ -run '^$$' -bench . -benchmem -count 3

cover: ## Run tests with coverage, print the total, open the HTML report
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out

keys: ## Rewrite the generated keybindings table in README.md from ui/keymap.go (make test fails when it is stale)
	go test ./ui -run TestReadmeKeyTableIsCurrent -update

deps-outdated: ## List direct dependencies with newer releases available
	@go list -u -m -f '{{if and .Update (not .Indirect)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all

setup: ## Point git at .githooks so pre-push runs the release gate
	git config core.hooksPath .githooks
	@echo "Git hooks configured. Pre-push will run 'make prepare-release'."

prepare-release: ## Pre-push gate: lint, vulnerabilities, licenses, tidy check, build, race test
	$(MAKE) lint vuln license-check publish-check
	go mod tidy -diff
	go build $(BUILD_FLAGS) ./...
	# -race, not plain test: the TUI runs handlers on gocui's loop goroutine, and a test calling one directly races the render loop. That class of bug is invisible without it.
	go test -race ./...
	go build $(BUILD_FLAGS) $(LDFLAGS) -o /tmp/$(BIN)-release-check . && rm -f /tmp/$(BIN)-release-check
	@echo "All checks passed. Ready to push."

release: ## Build a license-complete archive for the current platform
	$(MAKE) prepare-release
	$(MAKE) release-archive

# The platforms a release ships for. tcell and gocui are pure Go, so every one of these cross-compiles from any host.
RELEASE_PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

release-all: ## Build license-complete archives for every supported platform
	$(MAKE) prepare-release
	@set -eu; for p in $(RELEASE_PLATFORMS); do \
		GOOS="$${p%/*}" GOARCH="$${p#*/}" $(MAKE) release-archive; \
	done

# release-archive builds ONE platform's archive with no gate of its own: prepare-release runs `go test -race`, which cannot execute a cross-compiled binary, so the gate runs once on the host and the archives trust it.
# GOOS/GOARCH arrive via the environment; TARGET_GOOS/TARGET_GOARCH read `go env`, which honours them, so the archive name and the binary inside can never disagree.
# go-licenses is installed host-native (GOOS/GOARCH cleared for the install only): under `go run` the target's GOOS leaked into building the tool itself, which then could not exec on the host. Its `save` run keeps the target env so the analysed dependency set stays the target's.
release-archive:
	@if [ -e "$(DIST_DIR)/$(RELEASE_NAME).tar.gz" ]; then echo "refusing to overwrite $(DIST_DIR)/$(RELEASE_NAME).tar.gz"; exit 1; fi
	@set -eu; \
		mkdir -p "$(DIST_DIR)"; \
		archive="$(DIST_DIR)/$(RELEASE_NAME).tar.gz"; \
		archive_tmp="$(DIST_DIR)/.$(RELEASE_NAME).tar.gz.tmp"; \
		staging_root="$$(mktemp -d "$(DIST_DIR)/.release.XXXXXX")"; \
		trap 'rm -rf "$$staging_root"; rm -f "$$archive_tmp"' EXIT HUP INT TERM; \
		release_dir="$$staging_root/$(RELEASE_NAME)"; \
		mkdir -p "$$release_dir"; \
		go build $(BUILD_FLAGS) $(LDFLAGS) -o "$$release_dir/$(BIN)$(EXE_SUFFIX)" .; \
		cp LICENSE ACKNOWLEDGMENTS.md README.md SECURITY.md "$$release_dir/"; \
		cp -R LICENSES "$$release_dir/"; \
		GOOS= GOARCH= GOBIN="$(CURDIR)/$(DIST_DIR)/.tools" go install github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION); \
		"$(CURDIR)/$(DIST_DIR)/.tools/go-licenses" save . --save_path="$$release_dir/third_party_licenses"; \
		tar -C "$$staging_root" -czf "$$archive_tmp" "$(RELEASE_NAME)"; \
		mv "$$archive_tmp" "$$archive"; \
		echo "Built $$archive"

clean: ## Remove build and coverage artifacts
	@rm -f $(BIN) coverage.out
	@rm -rf bin/ dist/
