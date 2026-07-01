BINARY := swl
CMD    := ./cmd/swl

.PHONY: all build install test test-coverage test-pg test-mysql test-mssql clean

CGO_ENABLED ?= 1
export CGO_ENABLED

all: build

# Build swl in the repo root.
build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY) $(CMD)
	#upx $(BINARY)

# Install swl to $(go env GOPATH)/bin (or GOBIN if set).
install:
	go install $(CMD)

test:
	CGO_ENABLED=1 go test ./...

test-coverage:
	CGO_ENABLED=1 go test ./... -coverprofile=/tmp/swl-cover.out
	go tool cover -func=/tmp/swl-cover.out | tail -1

# Postgres integration tests (requires Docker).
test-pg:
	go test ./handler/pg/... -v -count=1

test-mysql:
	go test ./handler/mysql/... -v -count=1

test-mssql:
	go test ./handler/mssql/... -v -count=1

clean:
	rm -f $(BINARY)
