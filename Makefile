.PHONY: build build-pi test lint clean

BINARY      := turnstile-agent
PKG         := ./cmd/turnstile-agent
BIN_DIR     := bin
LDFLAGS     := -s -w

build:
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

build-pi:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(PKG)

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l .

clean:
	rm -rf $(BIN_DIR)
