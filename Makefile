.PHONY: all clean

BINARY_NAME=docker-inspector
INTERNAL_BINARY=internal-inspector

all: clean $(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME) cmd/docker-inspector/$(INTERNAL_BINARY)

# Build the internal Linux inspector first
cmd/docker-inspector/$(INTERNAL_BINARY): cmd/internal-inspector/main.go
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o cmd/docker-inspector/$(INTERNAL_BINARY) ./cmd/internal-inspector

# Build the main wrapper for the current platform
$(BINARY_NAME): cmd/docker-inspector/$(INTERNAL_BINARY)
	go build -o $(BINARY_NAME) ./cmd/docker-inspector

# Build for specific platforms
.PHONY: darwin linux windows
darwin: clean cmd/docker-inspector/$(INTERNAL_BINARY)
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin ./cmd/docker-inspector

linux: clean cmd/docker-inspector/$(INTERNAL_BINARY)
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux ./cmd/docker-inspector

windows: clean cmd/docker-inspector/$(INTERNAL_BINARY)
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME).exe ./cmd/docker-inspector