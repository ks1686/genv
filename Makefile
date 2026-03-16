.PHONY: build test lint fmt vet tidy clean

BINARY := gpm

build:
	go build -o $(BINARY) .

test:
	go test ./...

test-verbose:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) coverage.out coverage.html
