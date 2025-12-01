#!/usr/bin/make -f

.ONESHELL:
.SHELL := /usr/bin/bash

PROJECTNAME := $(shell basename "$$(pwd)")
PROJECTPATH := $(shell pwd)
FLAGS ?=

help:
	@echo "Usage: make [options] [arguments]\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

clean: ## Clean up build artifacts
	@go clean -cache -modcache -testcache -fuzzcache
	@find . -type d \( -name "go-build" -o -name ".gocache" -o -name ".gomodcache" \) -exec rm -rf {} + 2>/dev/null || true
	@rm -rf tmp || true

start: ## Start the application
	go mod tidy && \
	go build -o bin/lazyaws && \
	./bin/lazyaws

config: ## Open the config file
	@$(EDITOR) $(HOME)/.lazyaws/config.json

install: ## Install lazyaws binary to /usr/local/bin
	go build -o bin/lazyaws && \
	sudo mv bin/lazyaws /usr/local/bin/lazyaws
