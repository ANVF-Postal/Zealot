.DEFAULT_GOAL := build

# Git tags are the source of truth for releases. Between tags, show the
# distance from the latest tag and the abbreviated commit hash.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build
build:
	go build -ldflags "-X main.appVersion=$(VERSION)" -o zealot .
