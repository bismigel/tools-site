SHELL := /usr/bin/env bash
GO ?= go
SASS ?= sass
ADDR ?= :8080
BIN := bin/toolsd
THEME := theme/hugo-theme-stack
TOOL ?= slugify
TEXT ?= Hello, World!

.PHONY: all build css css-watch run dev list call test vet fmt-check check ci css-check vuln clean

all: build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN) ./cmd/toolsd

# Deterministic: pinned theme + copied palette -> theme/tools.css (docs/design/theme.md)
css:
	@set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	sed '/@import "custom.scss";/d' $(THEME)/assets/scss/style.scss > "$$tmp/entry.scss"; \
	cat theme/site-scss/custom.scss >> "$$tmp/entry.scss"; \
	$(SASS) --no-source-map --style=compressed --load-path=$(THEME)/assets/scss "$$tmp/entry.scss" theme/tools.css; \
	for tok in --rose-500 --accent-color; do \
	  grep -q -- "$$tok" theme/tools.css || { echo "make css: missing $$tok"; exit 1; }; \
	done; \
	echo "make css: theme/tools.css $$(wc -c < theme/tools.css) bytes"

css-watch:
	watchexec -w theme/site-scss -w theme/hugo-theme-stack/assets/scss -- $(MAKE) css

run: build
	$(BIN) -addr=$(ADDR)

dev:
	@scripts/dev.sh

list:
	curl -sS localhost$(ADDR)/api/tools | jq -r '.tools[].name'

call:
	curl -sS -X POST localhost$(ADDR)/api/tools/$(TOOL) \
	  -H 'content-type: application/json' \
	  -d '{"text": "$(TEXT)"}' | jq .

vet:
	$(GO) vet ./...

fmt-check:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test: vet
	$(GO) test -race ./...

css-check:
	@$(MAKE) --no-print-directory css
	@out=$$(git status --porcelain -- theme/tools.css); \
	if [ -n "$$out" ]; then echo "theme/tools.css is stale — run make css and commit it"; git diff --stat -- theme/tools.css; exit 1; fi

check: fmt-check test css-check build
	@echo "make check: green"

ci:
	@scripts/ci.sh

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean:
	rm -rf bin
