.DEFAULT_GOAL := build

# The latest release tag supplies the version base. Until the first tag exists,
# Zealot's next development line starts at v0.2.0.
VERSION_BASE := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.2.0)
EXACT_TAG := $(shell git describe --tags --exact-match HEAD 2>/dev/null)
BRANCH := $(shell git branch --show-current 2>/dev/null)
COMMIT_NUMBER := $(shell git rev-list --count HEAD 2>/dev/null || echo 1)
DEVCOM_NUMBER := $(shell printf "%04d" "$(COMMIT_NUMBER)")

# Exact release builds keep the tag verbatim. Otherwise main is marked as
# development and feature/PR branches receive a padded commit counter.
ifneq ($(EXACT_TAG),)
VERSION := $(EXACT_TAG)
else ifeq ($(BRANCH),main)
VERSION := $(VERSION_BASE)-dev
else
VERSION := $(VERSION_BASE)-DEVCOM-$(DEVCOM_NUMBER)
endif

.PHONY: build
build:
	go build -ldflags "-X main.appVersion=$(VERSION)" -o zealot .
