.DEFAULT_GOAL := build

# The latest release tag supplies the version base. Until the first tag exists,
# Zealot starts at v0.1.0.
VERSION_BASE ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.1.0)
BRANCH := $(shell git branch --show-current 2>/dev/null)
COMMIT_NUMBER := $(shell git rev-list --count HEAD 2>/dev/null || echo 1)
DEVCOM_NUMBER := $(shell printf "%04d" "$(COMMIT_NUMBER)")

# Main builds use the normal development suffix. Feature/PR branches get a
# monotonically increasing commit count, padded to at least four digits.
ifeq ($(BRANCH),main)
VERSION ?= $(VERSION_BASE)-dev
else
VERSION ?= $(VERSION_BASE)-DEVCOM-$(DEVCOM_NUMBER)
endif

.PHONY: build
build:
	go build -ldflags "-X main.appVersion=$(VERSION)" -o zealot .
