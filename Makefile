# Makefile for cross-compiling Go project

BINARY_NAME=surfboard
BUILD_DIR=build

PLATFORMS = \
	linux/amd64 \
	linux/mipsle  \
	linux/mips

all: clean $(PLATFORMS)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

linux/amd64: $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-amd64 .

# Для MIPS (little-endian)
linux/mipsle: $(BUILD_DIR)
	GOOS=linux GOARCH=mipsle go build -o $(BUILD_DIR)/$(BINARY_NAME)-mipsle .

# Для MIPS (big-endian)
linux/mips: $(BUILD_DIR)
	GOOS=linux GOARCH=mips go build -o $(BUILD_DIR)/$(BINARY_NAME)-mips .

clean:
	rm -rf $(BUILD_DIR)

.PHONY: all clean linux/amd64 linux/mipsle linux/mips
