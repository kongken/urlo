APP_NAME := urlo
BUILD_DIR := bin
GOOSE ?= goose

.PHONY: tidy proto proto-lint ent build build-migrate test-migrate test-deploy run clean postgres-migrate postgres-status

tidy:
	go mod tidy
	cd cmd/urlo-migrate && go mod tidy

proto:
	buf generate

proto-lint:
	buf lint

# Ent generator version is pinned in go.mod's tool directive.
ent:
	go generate ./internal/relations/ent

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/urlo

build-migrate:
	mkdir -p $(BUILD_DIR)
	cd cmd/urlo-migrate && go build -o ../../$(BUILD_DIR)/urlo-migrate .

test-migrate:
	cd cmd/urlo-migrate && go test -race ./...

test-deploy:
	python3 -m unittest discover -s scripts/tests -v

run:
	BUTTERFLY_CONFIG_TYPE=file \
	BUTTERFLY_CONFIG_FILE_PATH=./config.yaml \
	BUTTERFLY_TRACING_DISABLE=true \
	go run ./cmd/urlo

# Bundled, advisory-locked migration command; never invoked by Web startup.
postgres-migrate:
	@cd cmd/urlo-migrate && go run .

postgres-status:
	@test -n "$$URLO_POSTGRES_DSN" || (echo "URLO_POSTGRES_DSN is required" >&2; exit 1)
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$URLO_POSTGRES_DSN" $(GOOSE) -dir migrations/postgres status

clean:
	rm -rf $(BUILD_DIR)
