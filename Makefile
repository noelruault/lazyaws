#!/usr/bin/make -f

.ONESHELL:
.SHELL := /usr/bin/bash

PROJECTNAME := $(shell basename "$$(pwd)")
PROJECTPATH := $(shell pwd)
FLAGS ?=

EDITOR ?= code

help:
	@echo "Usage: make [options] [arguments]\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

clean: ## Clean up build artifacts
	@go clean -cache -modcache -testcache -fuzzcache
	@find . -type d \( -name "go-build" -o -name ".gocache" -o -name ".gomodcache" \) -exec rm -rf {} + 2>/dev/null || true
	@rm -rf ./web/dist || true

start: ## Start the application
	go mod tidy && go build -o lazyaws && ./lazyaws

config: ## Open the config file
	@sudo ${EDITOR:-vi} $(HOME)/.lazyaws/config.json
