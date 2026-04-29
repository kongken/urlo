APP_NAME := urlo
BUILD_DIR := bin

.PHONY: tidy proto proto-lint build run clean

tidy:
	go mod tidy

proto:
	buf generate

proto-lint:
	buf lint

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/urlo

run:
	BUTTERFLY_CONFIG_TYPE=file \
	BUTTERFLY_CONFIG_FILE_PATH=./config.yaml \
	BUTTERFLY_TRACING_DISABLE=true \
	go run ./cmd/urlo

clean:
	rm -rf $(BUILD_DIR)
