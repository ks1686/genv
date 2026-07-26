.PHONY: build test test-verbose test-cover cover-gate bench bench-gate ci lint fmt vet tidy clean

BINARY := genv

build:
	go build -o $(BINARY) .

test:
	go test ./...

# COVER_MIN is the statement-coverage floor enforced by cover-gate / make ci.
COVER_MIN ?= 80
# BENCH_MAX_MS is the cold-start budget for bench-gate (local default 200ms).
# Shared CI runners are slower/noisier; the workflow overrides this upward.
BENCH_MAX_MS ?= 200

# ci mirrors the GitHub Actions workflow — run this before pushing.
ci: vet
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "Unformatted files (run 'make fmt'):\n$$files"; exit 1; fi
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	$(MAKE) cover-gate
	$(MAKE) bench-gate

test-verbose:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# cover-gate fails when total statement coverage drops below COVER_MIN.
cover-gate:
	COVER_MIN=$(COVER_MIN) ./scripts/cover-gate.sh coverage.out

# bench runs all benchmarks once to verify they execute without error.
bench:
	go test -bench=. -benchtime=1x ./internal/resolver/...

# bench-gate enforces the cold-start budget for Detect + Resolve.
# Uses the worst of three BenchmarkDetect runs; fails if that exceeds BENCH_MAX_MS.
bench-gate:
	go test -bench=BenchmarkDetect -benchtime=5s -count=3 ./internal/resolver/ | tee /tmp/bench.txt
	@ms_int=$$(awk '/BenchmarkDetect/ { sub(/ns\/op/, "", $$3); if ($$3 > max) max = $$3 } END { printf "%d", max / 1000000 }' /tmp/bench.txt); \
	echo "BenchmarkDetect worst-case: $${ms_int}ms (budget $(BENCH_MAX_MS)ms)"; \
	if [ "$$ms_int" -gt "$(BENCH_MAX_MS)" ]; then echo "FAIL: cold-start budget exceeded (>$(BENCH_MAX_MS)ms)"; exit 1; fi

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
