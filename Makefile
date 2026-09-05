# Binaries the graders build live under the checkout rather than a shared
# TMPDIR name. Several worktrees of this module are often graded at once, and
# a fixed /tmp path let one checkout's build replace another's mid-run — which
# reads as a flaky implementation rather than as two builds sharing a name.
BINDIR := $(CURDIR)/build

.PHONY: all build test test-cover fmt vet lint tidy clean check corpus-guard oracle oracle-check conformance conformance-dialects

all: build

build:
	go build ./...

test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...

fmt:
	go tool gofumpt -l -w .

vet:
	go vet ./...

lint:
	go tool golangci-lint run

tidy:
	go mod tidy

clean:
	rm -f coverage.out
	rm -rf $(BINDIR)
	go clean

check: fmt vet test corpus-guard oracle-check

corpus-guard: ## Fail if the corpus has lost a case since the merge base with main
	@go run ./cmd/corpusguard

oracle: ## Regenerate docs/spec/measurements.md and the golden record from a live panel run
	go run ./cmd/oracle

oracle-check: ## Fail if the reference shells no longer behave as recorded
	go run ./cmd/oracle -check

conformance: ## Grade the core driver against bash over the whole corpus
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sh-under-test ./cmd/sh
	@go run ./cmd/oracle -bin $(BINDIR)/sh-under-test -binargs "-dialect bash" $(ARGS)

wild: ## Parse the shell scripts installed on this machine and report what fails
	@go run ./cmd/wild $(ARGS)

wild-run: ## Also RUN each script that parses, under both shells, and report where they disagree
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/wild-bash ./cmd/bash
	@go run ./cmd/wild -run $(BINDIR)/wild-bash $(ARGS)

conformance-dialects: ## Grade each dialect binary against the shell it claims to be
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/our-bash ./cmd/bash
	@go build -o $(BINDIR)/our-zsh ./cmd/zsh
	@go build -o $(BINDIR)/our-dash ./cmd/dash
	@go build -o $(BINDIR)/our-ksh ./cmd/ksh
	@go run ./cmd/oracle -bin $(BINDIR)/our-bash -against bash $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-zsh -against zsh $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-dash -against dash $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-ksh -against ksh93 $(ARGS)
