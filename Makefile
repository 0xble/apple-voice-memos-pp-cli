BINARY := apple-voice-memos-pp-cli
VERSION ?= dev

.PHONY: build test check install clean

build:
	mkdir -p bin
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) .

test:
	go test -race ./...

check:
	test -z "$$(gofmt -l .)"
	go test -race ./...
	go vet ./...
	govulncheck ./...
	@tmpdir=$$(mktemp -d); trap 'rm -rf "$$tmpdir"' EXIT; go build -o "$$tmpdir/$(BINARY)" .

install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/$(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -rf bin dist
