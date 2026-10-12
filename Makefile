APP_NAME := urlo
BUILD_DIR := bin
GOOSE ?= goose

.PHONY: tidy proto proto-lint build run clean postgres-migrate postgres-status

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

# Install the pinned goose CLI as described in docs/storage.md.
# Migrations run separately from Web pods; DSN comes from the environment.
postgres-migrate:
	@test -n "$$URLO_POSTGRES_DSN" || (echo "URLO_POSTGRES_DSN is required" >&2; exit 1)
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$URLO_POSTGRES_DSN" $(GOOSE) -dir migrations/postgres up

postgres-status:
	@test -n "$$URLO_POSTGRES_DSN" || (echo "URLO_POSTGRES_DSN is required" >&2; exit 1)
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$URLO_POSTGRES_DSN" $(GOOSE) -dir migrations/postgres status

clean:
	rm -rf $(BUILD_DIR)
