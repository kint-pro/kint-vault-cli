VERSION ?= 0.2.0
LDFLAGS := -X main.version=$(VERSION)
BINARY  := kint-vault

.PHONY: build test clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/kint-vault/

test:
	go test ./...

clean:
	rm -f $(BINARY)
