.PHONY: build clean install test run help

BINARY_NAME=dhcpd
BUILD_DIR=build
PLUGIN_BUILD_DIR=$(BUILD_DIR)/plugins
INSTALL_PREFIX=/usr/local
CONFIG_DIR=/etc/go-dhcpd
DATA_DIR=/var/lib/go-dhcpd
PLUGIN_DIR=$(INSTALL_PREFIX)/lib/go-dhcpd/plugins
LDFLAGS=-X 'github.com/greeneg/go-dhcpd/internal/config.DefaultPluginDir=$(PLUGIN_DIR)'

help:
	@echo "Available targets:"
	@echo "  build       - Build the dhcpd binary and bundled plugins"
	@echo "  clean       - Remove build artifacts"
	@echo "  install     - Install dhcpd, plugins and configuration (requires root)"
	@echo "  uninstall   - Uninstall dhcpd (requires root)"
	@echo "  test        - Run tests"
	@echo "  run         - Build and run dhcpd with example config"
	@echo "  deps        - Download dependencies"

deps:
	go mod download
	go mod tidy

build: deps
	@mkdir -p $(BUILD_DIR) $(PLUGIN_BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dhcpd
	go build -o $(PLUGIN_BUILD_DIR)/file-config.plugin ./cmd/plugins/file-config

clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)

install: build
	@echo "Installing dhcpd..."
	install -D -m 0755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PREFIX)/bin/$(BINARY_NAME)
	install -D -m 0755 -d $(CONFIG_DIR)
	install -D -m 0644 config.example.json5 $(CONFIG_DIR)/config.json5.example
	install -D -m 0755 -d $(DATA_DIR)
	install -D -m 0755 -d $(PLUGIN_DIR)
	install -D -m 0755 $(PLUGIN_BUILD_DIR)/file-config.plugin $(PLUGIN_DIR)/file-config.plugin
	@echo "Installation complete!"
	@echo "1. Edit $(CONFIG_DIR)/config.json5"
	@echo "2. Run: sudo $(INSTALL_PREFIX)/bin/$(BINARY_NAME) -config $(CONFIG_DIR)/config.json5"

uninstall:
	rm -f $(INSTALL_PREFIX)/bin/$(BINARY_NAME)
	rm -rf $(PLUGIN_DIR)
	@echo "Uninstall complete. Config and data directories preserved."


test:
	go test -v ./...

run: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -config config.example.json5 -stdout

run-dev: deps
	@mkdir -p $(PLUGIN_BUILD_DIR)
	go build -o $(PLUGIN_BUILD_DIR)/file-config.plugin ./cmd/plugins/file-config
	go run -ldflags "$(LDFLAGS)" ./cmd/dhcpd -config config.example.json5 -stdout

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet
	@echo "Linting complete"
