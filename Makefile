# Makefile for cross-compiling Go project

BINARY_NAME=surfboard
BUILD_DIR=build

PLATFORMS = \
	linux/amd64 \
	linux/mipsle

all: clean $(PLATFORMS)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

linux/amd64: $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-amd64 .

linux/mipsle: $(BUILD_DIR)
	GOOS=linux GOARCH=mipsle go build -o $(BUILD_DIR)/$(BINARY_NAME)-mipsle .

clean:
	rm -rf $(BUILD_DIR)

.PHONY: all clean linux/amd64 linux/mipsle
