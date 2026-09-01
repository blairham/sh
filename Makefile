.PHONY: all build test test-cover fmt vet lint tidy clean check oracle oracle-check conformance conformance-dialects

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
	go clean

check: fmt vet test oracle-check

oracle: ## Regenerate docs/spec/measurements.md and the golden record from a live panel run
	go run ./cmd/oracle

oracle-check: ## Fail if the reference shells no longer behave as recorded
	go run ./cmd/oracle -check

conformance: ## Grade the core driver against bash over the whole corpus
	@go build -o $${TMPDIR:-/tmp}/sh-under-test ./cmd/sh
	@go run ./cmd/oracle -bin $${TMPDIR:-/tmp}/sh-under-test -binargs "-dialect bash" $(ARGS)

wild: ## Parse the shell scripts installed on this machine and report what fails
	@go run ./cmd/wild $(ARGS)

wild-run: ## Also RUN each script that parses, under both shells, and report where they disagree
	@go build -o $${TMPDIR:-/tmp}/wild-bash ./cmd/bash
	@go run ./cmd/wild -run $${TMPDIR:-/tmp}/wild-bash $(ARGS)

conformance-dialects: ## Grade each dialect binary against the shell it claims to be
	@go build -o $${TMPDIR:-/tmp}/our-bash ./cmd/bash
	@go build -o $${TMPDIR:-/tmp}/our-zsh ./cmd/zsh
	@go build -o $${TMPDIR:-/tmp}/our-dash ./cmd/dash
	@go build -o $${TMPDIR:-/tmp}/our-ksh ./cmd/ksh
	@go run ./cmd/oracle -bin $${TMPDIR:-/tmp}/our-bash -against bash $(ARGS)
	@go run ./cmd/oracle -bin $${TMPDIR:-/tmp}/our-zsh -against zsh $(ARGS)
	@go run ./cmd/oracle -bin $${TMPDIR:-/tmp}/our-dash -against dash $(ARGS)
	@go run ./cmd/oracle -bin $${TMPDIR:-/tmp}/our-ksh -against ksh93 $(ARGS)
