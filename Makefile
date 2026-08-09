BINARY := bin/jinja-fmt
VERSION ?= 0.1.0
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
GOCACHE ?= /tmp/jinja-fmt-go-cache

.PHONY: build test cross clean

build:
	mkdir -p bin
	GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/jinja-fmt

test:
	GOCACHE=$(GOCACHE) go test -race ./...
	GOCACHE=$(GOCACHE) go vet ./...
	git diff --check

cross:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-linux-amd64 ./cmd/jinja-fmt
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-linux-arm64 ./cmd/jinja-fmt
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-darwin-amd64 ./cmd/jinja-fmt
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-darwin-arm64 ./cmd/jinja-fmt
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-windows-amd64.exe ./cmd/jinja-fmt
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/jinja-fmt-windows-arm64.exe ./cmd/jinja-fmt

clean:
	rm -rf bin dist

