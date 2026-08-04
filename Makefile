VERSION := $(shell cat VERSION)
BINARY := freemodel
DIST := dist

.PHONY: build build-all test lint clean docker

build:
	go build -o $(DIST)/$(BINARY) ./cmd/freemodel

docker:
	go build -o freemodel-router ./cmd/freemodel

build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 go build -o $(DIST)/$(BINARY)-darwin-amd64 ./cmd/freemodel
	GOOS=darwin GOARCH=arm64 go build -o $(DIST)/$(BINARY)-darwin-arm64 ./cmd/freemodel
	GOOS=linux GOARCH=amd64 go build -o $(DIST)/$(BINARY)-linux-amd64 ./cmd/freemodel
	GOOS=linux GOARCH=arm64 go build -o $(DIST)/$(BINARY)-linux-arm64 ./cmd/freemodel
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/$(BINARY)-windows-amd64.exe ./cmd/freemodel
	@echo "Done. Version: $(VERSION)"

test:
	go test ./... -v

lint:
	go vet ./...

clean:
	rm -rf $(DIST)
