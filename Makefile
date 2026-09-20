.PHONY: build clean install test run help

BINARY_NAME=dhcpd
BUILD_DIR=build
INSTALL_PREFIX=/usr/local
CONFIG_DIR=/etc/go-dhcpd
DATA_DIR=/var/lib/go-dhcpd

help:
	@echo "Available targets:"
	@echo "  build       - Build the dhcpd binary"
	@echo "  clean       - Remove build artifacts"
	@echo "  install     - Install dhcpd and configuration (requires root)"
	@echo "  uninstall   - Uninstall dhcpd (requires root)"
	@echo "  test        - Run tests"
	@echo "  run         - Build and run dhcpd with example config"
	@echo "  deps        - Download dependencies"

deps:
	go mod download
	go mod tidy

build: deps
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dhcpd

clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)

install: build
	@echo "Installing dhcpd..."
	install -D -m 0755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PREFIX)/bin/$(BINARY_NAME)
	install -D -m 0755 -d $(CONFIG_DIR)
	install -D -m 0644 config.example.json5 $(CONFIG_DIR)/config.json5.example
	install -D -m 0755 -d $(DATA_DIR)
	@echo "Installation complete!"
	@echo "1. Edit $(CONFIG_DIR)/config.json5"
	@echo "2. Run: sudo $(INSTALL_PREFIX)/bin/$(BINARY_NAME) -config $(CONFIG_DIR)/config.json5"

uninstall:
	rm -f $(INSTALL_PREFIX)/bin/$(BINARY_NAME)
	@echo "Uninstall complete. Config and data directories preserved."

test:
	go test -v ./...

run: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -config config.example.json5 -stdout

run-dev:
	go run ./cmd/dhcpd -config config.example.json5 -stdout

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet
	@echo "Linting complete"
