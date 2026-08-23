MAKEFLAGS += --no-print-directory

SPOOL_STRESS_TIMEOUT ?= 10m

.PHONY: bench build check fuzz golangci lint staticcheck stress test test-race tidy tidy-check fmt fmt-check vet vulncheck clean

build: check
	go build ./...
	mkdir -p dist
	go build -o dist/spl ./cmd/spl

check: tidy-check lint test test-race vulncheck

test:
	go test ./...
	go test ./cmd/spl/...

test-race:
	go test -race ./...
	go test -race ./cmd/spl/...

fuzz:
	go test -run='^$$' -fuzz=FuzzPersistedRepositoryValidation -fuzztime=5s ./internal/repository

stress:
	SPOOL_STRESS=1 go test -count=1 -timeout=$(SPOOL_STRESS_TIMEOUT) -run='^TestStress' ./internal/repository ./internal/resolve

bench:
	go test -run='^$$' -bench=. -benchmem ./...
	go test -run='^$$' -bench=. -benchmem ./cmd/spl/...

tidy:
	go work sync
	go mod tidy
	cd cmd/spl && go mod tidy

tidy-check:
	go mod tidy -diff
	cd cmd/spl && go mod tidy -diff

fmt:
	gofmt -l -w .

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...
	go vet ./cmd/spl/...

staticcheck:
	staticcheck ./...
	staticcheck ./cmd/spl/...

golangci:
	golangci-lint run ./...
	golangci-lint run ./cmd/spl/...

vulncheck:
	govulncheck ./...
	govulncheck ./cmd/spl/...

lint: fmt-check vet staticcheck golangci

clean:
	rm -rf bin dist
