.PHONY: build test clean

# Extract version from Git, fall back to "devel" if Git is not initialized
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "devel")
LDFLAGS := -X main.Version=$(VERSION)

build:
	@echo "Building leetgptsolver version $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o leetgptsolver .

clean:
	rm -f leetgptsolver

test:
	go test ./...
