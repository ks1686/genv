.PHONY: build test test-verbose test-cover ci lint fmt vet tidy clean

BINARY := genv

build:
	go build -o $(BINARY) .

test:
	go test ./...

# ci mirrors the GitHub Actions workflow — run this before pushing.
ci: vet
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "Unformatted files (run 'make fmt'):\n$$files"; exit 1; fi
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

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
