.PHONY: all build test test-cover fmt vet lint tidy clean check oracle oracle-check

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
