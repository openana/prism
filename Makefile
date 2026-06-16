MODULE  := github.com/openana/prism
BINARY  := prism
VERSION := $(shell cat .version 2>/dev/null || echo "N/A")
HASH    := $(shell git rev-parse HEAD 2>/dev/null || echo "0000000000000000000000000000000000000000")
DATE    := $(shell date -u +'%Y-%m-%d %H:%M:%S+00:00')
GOOS    := $(shell go env GOOS)
GOARCH  := $(shell go env GOARCH)
GOVER   := $(shell go env GOVERSION)

LDFLAGS := -s -w \
	-X '$(MODULE)/pkg/meta.Version=$(VERSION)' \
	-X '$(MODULE)/pkg/meta.CommitHash=$(HASH)' \
	-X '$(MODULE)/pkg/meta.BuildDate=$(DATE)' \
	-X '$(MODULE)/pkg/meta.Platform=$(GOOS)/$(GOARCH)' \
	-X '$(MODULE)/pkg/meta.GoVersion=$(GOVER)'

.PHONY: build test bench clean update-cname
.DEFAULT_GOAL := build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/prism/

test:
	go test ./...

bench:
	go test -bench=. -benchmem ./...

clean:
	rm -f $(BINARY)

update-cname:
	python3 pkg/mirrors/cname/convert.py
