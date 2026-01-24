.PHONY: build clean install test

BINARY_NAME=open-agents
BUILD_DIR=build

# Build for current platform
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/open-agents

# Build for all platforms
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/open-agents
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/open-agents

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/open-agents
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/open-agents

build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/open-agents

# Install to /usr/local/bin
install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Run tests
test:
	go test -v ./...

# Download dependencies
deps:
	go mod download
	go mod tidy
