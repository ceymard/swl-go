BINARY := swl
CMD    := ./cmd/swl

.PHONY: all build install test test-coverage test-pg clean

all: build

# Build swl in the repo root.
build:
	go build -o $(BINARY) $(CMD)

# Install swl to $(go env GOPATH)/bin (or GOBIN if set).
install:
	go install $(CMD)

test:
	go test ./...

test-coverage:
	go test ./... -coverprofile=/tmp/swl-cover.out
	go tool cover -func=/tmp/swl-cover.out | tail -1

# Postgres integration tests (requires Docker).
test-pg:
	go test ./handler/pg/... -v -count=1

clean:
	rm -f $(BINARY)
