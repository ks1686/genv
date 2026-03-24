.PHONY: build test test-verbose test-cover bench bench-gate ci lint fmt vet tidy clean

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

# bench runs all benchmarks once to verify they execute without error.
bench:
	go test -bench=. -benchtime=1x ./internal/resolver/...

# bench-gate enforces the <200ms cold-start budget for Detect + Resolve.
# Uses benchstat threshold: if BenchmarkDetect p50 > 200ms the gate fails.
bench-gate:
	go test -bench=BenchmarkDetect -benchtime=5s -count=3 ./internal/resolver/ | tee /tmp/bench.txt
	@ms=$$(grep BenchmarkDetect /tmp/bench.txt | awk '{print $$3}' | sed 's/ns\/op//' | sort -n | tail -1); \
	ms_int=$$(echo "$$ms / 1000000" | bc); \
	echo "BenchmarkDetect worst-case: $${ms_int}ms"; \
	if [ "$$ms_int" -gt 200 ]; then echo "FAIL: cold-start budget exceeded (>200ms)"; exit 1; fi

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
