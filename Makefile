.PHONY: all build test test-cover fmt vet lint tidy clean check

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

check: fmt vet test
