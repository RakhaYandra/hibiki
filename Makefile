.PHONY: build release lint vet test race e2e clean install

PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/RakhaYandra/hibiki/internal/cli.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/hibiki ./cmd/hibiki

release:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/hibiki-linux-amd64 ./cmd/hibiki
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/hibiki-linux-arm64 ./cmd/hibiki
	sha256sum dist/hibiki-linux-* > dist/SHA256SUMS

lint:
	test -z "$$(gofmt -l cmd internal test)" || (gofmt -l cmd internal test && exit 1)
	go vet ./...

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

e2e:
	go test -count=1 ./test/integration/

install: build
	install -Dm755 bin/hibiki $(PREFIX)/bin/hibiki

clean:
	rm -rf bin/ dist/
