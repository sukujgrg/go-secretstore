CGO_ENABLED ?= 1
GO         ?= go
BIN_DIR    := bin
BIN        := $(BIN_DIR)/secretstore

GOLANGCI_LINT ?= golangci-lint

.PHONY: all build test test-native lint vet fmt clean smoke

all: build

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -o $(BIN) ./cmd/secretstore

test:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./...

test-native:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test -tags secretstore_native -count=1 ./...

lint:
	CGO_ENABLED=$(CGO_ENABLED) $(GOLANGCI_LINT) run ./...
	GOOS=windows CGO_ENABLED=0 $(GOLANGCI_LINT) run ./...

vet:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) vet ./...

fmt:
	$(GO) fmt ./...

smoke: build
	$(BIN) set go-secretstore.smoke local-test smoke-value
	$(BIN) get go-secretstore.smoke local-test
	$(BIN) delete go-secretstore.smoke local-test

clean:
	rm -rf $(BIN_DIR)
