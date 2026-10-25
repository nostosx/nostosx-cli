GO ?= go
GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
export GO
export GOWORK := off
.DEFAULT_GOAL := check

MODULES := .

# Each repository owns this file so a standalone checkout has no shared tooling dependency.
define in_modules
	@set -eu; for module in $(MODULES); do \
		printf '\n==> %s: %s\n' "$$module" "$@"; \
		(cd "$$module"; $(1)); \
	done
endef

.PHONY: all help check fmt fmt-check fix modernize-check tidy tidy-check vet build test race cover lint vulncheck clean

all: check

help:
	@printf '%s\n' \
	  'make check       Check CLI and four generated application variants' \
	  'make test        CLI tests and generated application tests' \
	  'make race        CLI and generated application tests with the race detector' \
	  'make lint        Lint normal and integration builds (golangci-lint v2.15.0)' \
	  'make vulncheck   Scan both builds (govulncheck v1.9.0)' \
	  'make smoke       Run all four generated variants with loopback listeners and local model fixtures' \
	  'make templates-lint       Lint all four generated variants' \
	  'make templates-vulncheck  Scan all four generated variants' \
	  'make fmt/fix/tidy Apply formatting, modernization or dependency changes' \
	  'make cover       Race coverage in coverage.out per module'

fmt:
	$(call in_modules,"$(GOFMT)" -w .)

fmt-check:
	$(call in_modules,out=$$("$(GOFMT)" -l .); if [ -n "$$out" ]; then printf 'gofmt needed:\n%s\n' "$$out"; exit 1; fi)

fix:
	$(call in_modules,for tags in '' integration; do "$(GO)" fix -tags "$$tags" ./...; done)

modernize-check:
	$(call in_modules,for tags in '' integration; do out=$$("$(GO)" fix -tags "$$tags" -diff ./...); if [ -n "$$out" ]; then printf '%s\n' "$$out"; exit 1; fi; done)

tidy:
	$(call in_modules,"$(GO)" mod tidy)

tidy-check:
	$(call in_modules,out=$$("$(GO)" mod tidy -diff); if [ -n "$$out" ]; then printf '%s\n' "$$out"; exit 1; fi)

vet:
	$(call in_modules,for tags in '' integration; do "$(GO)" vet -tags "$$tags" ./...; done)

build:
	$(call in_modules,for tags in '' integration; do "$(GO)" build -tags "$$tags" ./...; done)

test:
	$(call in_modules,"$(GO)" test -tags integration -shuffle=on -count=1 ./...)

race:
	$(call in_modules,"$(GO)" test -tags integration -race -shuffle=on -count=1 ./...)

cover:
	$(call in_modules,"$(GO)" test -race -shuffle=on -count=1 -tags integration -coverprofile=coverage.out -covermode=atomic ./...; "$(GO)" tool cover -func=coverage.out)

lint:
	$(call in_modules,for tags in '' integration; do "$(GOLANGCI_LINT)" run --config="$(CURDIR)/.golangci.yml" --build-tags="$$tags" ./...; done)

vulncheck:
	$(call in_modules,for tags in '' integration; do "$(GOVULNCHECK)" -tags "$$tags" ./...; done)

clean:
	$(call in_modules,rm -f coverage.out coverage.txt)

check: fmt-check modernize-check tidy-check vet build race

.PHONY: smoke templates-lint templates-vulncheck
smoke:
	NOSTOSX_GENERATED_SMOKE=1 "$(GO)" test -tags integration -race -count=1 -timeout 20m -v -run '^TestGeneratedSmoke$$' ./internal/scaffold

templates-lint:
	GOLANGCI_LINT="$(GOLANGCI_LINT)" NOSTOSX_TEMPLATE_GATE=lint "$(GO)" test -tags integration -count=1 -v -run '^TestTemplateGates$$' ./internal/scaffold

templates-vulncheck:
	GOVULNCHECK="$(GOVULNCHECK)" NOSTOSX_TEMPLATE_GATE=vulncheck "$(GO)" test -tags integration -count=1 -v -run '^TestTemplateGates$$' ./internal/scaffold
