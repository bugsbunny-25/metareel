SHELL        := /usr/bin/env bash
APP_NAME     := metareel
BIN_DIR      := bin
BIN          := $(BIN_DIR)/$(APP_NAME)
TOOLS_DIR    := tools
TOOLS_BIN_DIR := .tools/bin
DLV_BIN      := $(TOOLS_BIN_DIR)/dlv
AIR_BIN      := $(TOOLS_BIN_DIR)/air
ASYNQMON_PORT ?= 7070
DLV_PORT      ?= 2345
SERVER_PORT   ?= 7080
REDIS_ADDR    ?= 127.0.0.1:6379
DEVREDIS_PID  := tmp/devredis.pid
DEVREDIS_LOG  := tmp/devredis.log
ASYNQMON_PID  := tmp/asynqmon.pid
ASYNQMON_LOG  := tmp/asynqmon.log

# Tool versions (installed via `go install`).
SQLC_VERSION    := v1.29.0
GOOSE_VERSION   := v3.25.0

GO           ?= go
DOCKER       ?= docker
COMPOSE      ?= docker compose
COVERAGE_FILE ?= coverage.out
REPORT_DIR   ?= tmp/reports

# Pull DB settings from .env if present (for goose CLI commands).
ifneq (,$(wildcard .env))
	include .env
	export
endif
DATABASE_URL             ?= file:./data/metareel.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)
DATABASE_PATH            ?= ./data/metareel.db
DATABASE_MIGRATIONS_DIR  ?= db/migrations
DATABASE_DIR             := $(dir $(DATABASE_PATH))

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_.-]+:.*?## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## ------- Local App -------

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: run
run: dev-prepare ## Run the API locally (reads .env if present)
	$(GO) run ./cmd/server

.PHONY: build
build: ## Build local binary
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

.PHONY: test
test: ## Run tests
	$(GO) test ./... -race -count=1

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

## ------- CI Parity (Local + GitHub Actions) -------

.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	$(GO) mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: test-cover
test-cover: ## Run tests with race detector + coverage profile
	$(GO) test ./... -race -covermode=atomic -coverprofile=$(COVERAGE_FILE) -count=1

.PHONY: coverage-total
coverage-total: ## Print total coverage percentage from $(COVERAGE_FILE)
	@$(GO) tool cover -func=$(COVERAGE_FILE) | awk '/^total:/{print $$3}'

.PHONY: ci-local
ci-local: tidy-check vet build test-cover ## Run the same core checks as CI go-quality job
	@echo "ci-local completed. coverage file: $(COVERAGE_FILE)"

.PHONY: integration-local
integration-local: ## Run integration-oriented tests with local Redis + SQLite env
	@mkdir -p tmp data
	REDIS_ADDR="$(REDIS_ADDR)" \
	REDIS_PASSWORD="" \
	REDIS_DB="0" \
	DATABASE_URL="file:./tmp/integration.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)" \
	DATABASE_MIGRATIONS_DIR="db/migrations" \
	APP_ENV="test" \
	$(GO) test ./... -count=1

.PHONY: security-local
security-local: ## Run govulncheck without requiring a preinstalled binary
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: codeql-local
codeql-local: ## Run local CodeQL scan (requires codeql CLI and query packs)
	@if ! command -v codeql >/dev/null 2>&1; then \
		echo "codeql CLI is not installed. Install it and re-run make codeql-local."; \
		echo "Tip: use make security-local for lightweight local security checks."; \
		exit 1; \
	fi
	rm -rf tmp/codeql-db
	codeql database create tmp/codeql-db --language=go --source-root=.
	codeql database analyze tmp/codeql-db codeql/go-queries:codeql-suites/go-security-and-quality.qls --format=sarif-latest --output=tmp/codeql.sarif
	@echo "CodeQL SARIF written to tmp/codeql.sarif"

## ------- Reports (Local + CI artifacts) -------

.PHONY: reports-clean
reports-clean: ## Remove generated reports artifacts
	rm -rf "$(REPORT_DIR)"
	rm -rf tmp/ci-artifacts

.PHONY: reports-local
reports-local: ## Generate local HTML/JUnit/JSON reports under $(REPORT_DIR)
	@mkdir -p "$(REPORT_DIR)"
	@set -euo pipefail; \
	echo "writing reports to: $(REPORT_DIR)"; \
	echo ""; \
	echo "==> tidy-check"; \
	$(MAKE) tidy-check; \
	echo "==> vet"; \
	$(MAKE) vet; \
	echo "==> build"; \
	$(MAKE) build; \
	echo "==> tests (junit.xml, test.json, coverage.out)"; \
	$(GO) run gotest.tools/gotestsum@latest \
		--format standard-verbose \
		--junitfile "$(REPORT_DIR)/junit.xml" \
		--jsonfile "$(REPORT_DIR)/test.json" \
		-- \
		./... -race -covermode=atomic -coverprofile="$(REPORT_DIR)/coverage.out" -count=1; \
	echo "==> coverage html"; \
	$(GO) tool cover -html="$(REPORT_DIR)/coverage.out" -o "$(REPORT_DIR)/coverage.html"; \
	echo "==> govulncheck (govulncheck.txt)"; \
	govuln_status=0; \
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./... 2>&1 | tee "$(REPORT_DIR)/govulncheck.txt" || govuln_status=$$?; \
	echo "==> report index (index.html)"; \
	{ \
		echo '<!doctype html>'; \
		echo '<html lang="en"><head><meta charset="utf-8" />'; \
		echo '<meta name="viewport" content="width=device-width,initial-scale=1" />'; \
		echo '<title>metareel local reports</title>'; \
		echo '<style>body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Arial,sans-serif;max-width:900px;margin:24px auto;padding:0 16px}code{background:#f4f4f4;padding:2px 6px;border-radius:4px}a{color:#0366d6;text-decoration:none}a:hover{text-decoration:underline}li{margin:8px 0}</style>'; \
		echo '</head><body>'; \
		echo '<h2>metareel reports</h2>'; \
		echo '<p>Generated by <code>make reports-local</code></p>'; \
		echo '<ul>'; \
		echo '<li><a href="coverage.html">Coverage (HTML)</a> (<code>coverage.out</code>)</li>'; \
		echo '<li><a href="junit.xml">Tests (JUnit XML)</a></li>'; \
		echo '<li><a href="test.json">Tests (JSON log)</a></li>'; \
		echo '<li><a href="govulncheck.txt">govulncheck output</a></li>'; \
		if [ -f "tmp/codeql.sarif" ]; then echo '<li><a href="../codeql.sarif">CodeQL SARIF (tmp/codeql.sarif)</a></li>'; fi; \
		echo '</ul>'; \
		echo '<p>Note: this index is a static file browser. Open it in your OS browser.</p>'; \
		echo '</body></html>'; \
	} >"$(REPORT_DIR)/index.html"; \
	echo ""; \
	echo "Open: $(REPORT_DIR)/index.html"; \
	exit $$govuln_status

.PHONY: reports-open
reports-open: ## Open $(REPORT_DIR)/index.html in your browser
	@if [ ! -f "$(REPORT_DIR)/index.html" ]; then \
		echo "missing $(REPORT_DIR)/index.html; run: make reports-local"; \
		exit 1; \
	fi
	@{ command -v open >/dev/null 2>&1 && open "$(REPORT_DIR)/index.html"; } || \
	 { command -v xdg-open >/dev/null 2>&1 && xdg-open "$(REPORT_DIR)/index.html"; } || \
	 { echo "open this file in a browser: $(REPORT_DIR)/index.html"; }

.PHONY: ci-artifacts
ci-artifacts: ## Generate CI-friendly artifacts under tmp/ci-artifacts (HTML/JUnit/JSON/govulncheck)
	@mkdir -p tmp/ci-artifacts
	@set -euo pipefail; \
	$(GO) tool cover -html="$(COVERAGE_FILE)" -o tmp/ci-artifacts/coverage.html; \
	$(GO) run gotest.tools/gotestsum@latest \
		--format standard-verbose \
		--junitfile tmp/ci-artifacts/junit.xml \
		--jsonfile tmp/ci-artifacts/test.json \
		-- \
		./... -race -count=1; \
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./... 2>&1 | tee tmp/ci-artifacts/govulncheck.txt

## ------- Local Dev Services -------

.PHONY: dev-prepare
dev-prepare: ## Create required local runtime directories
	@mkdir -p tmp
	@mkdir -p "$(DATABASE_DIR)"
	@mkdir -p "$(TOOLS_BIN_DIR)"

.PHONY: dev-dlv-install
dev-dlv-install: dev-tools-install ## Backward-compatible alias to install local tools

.PHONY: dev-tools-install
dev-tools-install: dev-prepare ## Build local Air + Delve binaries used by dev workflows
	@if [ ! -x "$(AIR_BIN)" ]; then \
		echo "installing local air to $(AIR_BIN)"; \
		$(GO) build -modfile=$(TOOLS_DIR)/go.mod -o "$(AIR_BIN)" github.com/air-verse/air; \
	else \
		echo "local air already installed at $(AIR_BIN)"; \
	fi
	@if [ ! -x "$(DLV_BIN)" ]; then \
		echo "installing local dlv to $(DLV_BIN)"; \
		$(GO) build -modfile=$(TOOLS_DIR)/go.mod -o "$(DLV_BIN)" github.com/go-delve/delve/cmd/dlv; \
	else \
		echo "local dlv already installed at $(DLV_BIN)"; \
	fi

.PHONY: dev-doctor
dev-doctor: dev-tools-install ## Validate local dev run/debug prerequisites
	@set -euo pipefail; \
	echo "== toolchain =="; \
	echo "air: $(AIR_BIN)"; \
	"$(AIR_BIN)" -v >/dev/null || true; \
	echo "dlv: $(DLV_BIN)"; \
	"$(DLV_BIN)" version | sed -n '1,2p'; \
	echo ""; \
	echo "== ports =="; \
	if lsof -nP -iTCP:"$(SERVER_PORT)" -sTCP:LISTEN >/dev/null 2>&1; then \
		echo "api port $(SERVER_PORT): in use"; \
	else \
		echo "api port $(SERVER_PORT): free"; \
	fi; \
	if lsof -nP -iTCP:"$(DLV_PORT)" -sTCP:LISTEN >/dev/null 2>&1; then \
		echo "debug port $(DLV_PORT): in use"; \
	else \
		echo "debug port $(DLV_PORT): free"; \
	fi; \
	if lsof -nP -iTCP:"$(ASYNQMON_PORT)" -sTCP:LISTEN >/dev/null 2>&1; then \
		echo "asynqmon port $(ASYNQMON_PORT): in use"; \
	else \
		echo "asynqmon port $(ASYNQMON_PORT): free"; \
	fi; \
	echo ""; \
	echo "== services =="; \
	if [ -f "$(DEVREDIS_PID)" ] && kill -0 "$$(cat "$(DEVREDIS_PID)")" 2>/dev/null; then \
		echo "devredis: running (pid $$(cat "$(DEVREDIS_PID)"))"; \
	else \
		echo "devredis: not running"; \
	fi; \
	if [ -f "$(ASYNQMON_PID)" ] && kill -0 "$$(cat "$(ASYNQMON_PID)")" 2>/dev/null; then \
		echo "asynqmon: running (pid $$(cat "$(ASYNQMON_PID)"))"; \
	else \
		echo "asynqmon: not running"; \
	fi; \
	echo ""; \
	echo "recommended flows:"; \
	echo "  make dev-up        # run profile"; \
	echo "  make dev-up-debug  # debug profile + attach"; \
	echo "  make dev-down      # stop all local dev services"

.PHONY: dev-up
dev-up: ## One-shot local dev startup (run mode: no debugger)
	@$(MAKE) dev-prepare
	@$(MAKE) dev-redis-up
	@$(MAKE) dev-asynqmon-up
	@echo "starting API with hot-reload (run profile)..."
	@$(MAKE) dev-server-air-run

.PHONY: dev-up-debug
dev-up-debug: ## One-shot local dev startup (debug mode: hot-reload + Delve)
	@$(MAKE) dev-prepare
	@$(MAKE) dev-redis-up
	@$(MAKE) dev-asynqmon-up
	@echo "starting API with hot-reload (debug profile)..."
	@$(MAKE) dev-server-air-debug

.PHONY: dev-down
dev-down: ## Stop background local dev services started by Makefile
	@$(MAKE) dev-air-down
	@$(MAKE) dev-debugger-down
	@$(MAKE) dev-api-down
	@$(MAKE) dev-asynqmon-down
	@$(MAKE) dev-redis-down

.PHONY: dev-air-down
dev-air-down: ## Stop local Air process for this workspace
	@stopped=0; \
	for PID in $$(pgrep -f "air.*\\.air\\.(run|debug)\\.toml|github.com/air-verse/air" 2>/dev/null); do \
		CMD="$$(ps -p "$$PID" -o command= 2>/dev/null || true)"; \
		case "$$CMD" in \
			*metareel*air*|*".air.run.toml"*|*".air.debug.toml"*) \
				kill "$$PID" 2>/dev/null || true; \
				echo "stopped air (pid $$PID)"; \
				stopped=1; \
				;; \
		esac; \
	done; \
	if [ "$$stopped" -eq 0 ]; then \
		echo "air not running for this workspace"; \
	fi

.PHONY: dev-debugger-down
dev-debugger-down: ## Stop local Delve debugger (used by dev-up/air)
	@stopped=0; \
	for PID in $$(lsof -nP -iTCP:"$(DLV_PORT)" -sTCP:LISTEN -t 2>/dev/null); do \
		CMD="$$(ps -p "$$PID" -o command= 2>/dev/null || true)"; \
		case "$$CMD" in \
			*dlv*) \
				kill "$$PID" 2>/dev/null || true; \
				echo "stopped debugger (pid $$PID)"; \
				stopped=1; \
				;; \
		esac; \
	done; \
	if [ "$$stopped" -eq 0 ]; then \
		echo "debugger not running on 127.0.0.1:$(DLV_PORT)"; \
	fi

.PHONY: dev-api-down
dev-api-down: ## Stop local API process bound to SERVER_PORT
	@stopped=0; \
	for PID in $$(lsof -nP -iTCP:"$(SERVER_PORT)" -sTCP:LISTEN -t 2>/dev/null); do \
		CMD="$$(ps -p "$$PID" -o command= 2>/dev/null || true)"; \
		case "$$CMD" in \
			*metareel*|*tmp/server*|*__debug_bin*|*dlv*) \
				kill "$$PID" 2>/dev/null || true; \
				echo "stopped api (pid $$PID)"; \
				stopped=1; \
				;; \
		esac; \
	done; \
	if [ "$$stopped" -eq 0 ]; then \
		echo "api not running on localhost:$(SERVER_PORT)"; \
	fi

.PHONY: dev-redis-up
dev-redis-up: ## Start Go-based devredis in background (no Docker)
	@mkdir -p tmp
	@if [ -f "$(DEVREDIS_PID)" ] && kill -0 "$$(cat "$(DEVREDIS_PID)")" 2>/dev/null; then \
		echo "devredis already running (pid $$(cat "$(DEVREDIS_PID)"))"; \
		exit 0; \
	fi
	nohup $(GO) -C $(TOOLS_DIR) run ./cmd/devredis --addr="$(REDIS_ADDR)" >"$(DEVREDIS_LOG)" 2>&1 & echo $$! > "$(DEVREDIS_PID)"
	@sleep 1
	@if ! kill -0 "$$(cat "$(DEVREDIS_PID)")" 2>/dev/null; then \
		echo "failed to start devredis, see $(DEVREDIS_LOG)"; \
		rm -f "$(DEVREDIS_PID)"; \
		exit 1; \
	fi
	@echo "devredis starting on $(REDIS_ADDR) (pid $$(cat "$(DEVREDIS_PID)"))"
	@echo "logs: $(DEVREDIS_LOG)"

.PHONY: dev-redis-down
dev-redis-down: ## Stop Go-based devredis started by Makefile
	@stopped=0; \
	if [ -f "$(DEVREDIS_PID)" ]; then \
		PID="$$(cat "$(DEVREDIS_PID)")"; \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID"; \
			echo "stopped devredis (pid $$PID)"; \
			stopped=1; \
		else \
			echo "stale pid file found for devredis"; \
		fi; \
	fi; \
	REDIS_TARGET="$(REDIS_ADDR)"; \
	HOST="$${REDIS_TARGET%:*}"; \
	PORT="$${REDIS_TARGET##*:}"; \
	for PID in $$(lsof -nP -iTCP:"$$PORT" -sTCP:LISTEN -t 2>/dev/null); do \
		CMD="$$(ps -p "$$PID" -o command= 2>/dev/null || true)"; \
		case "$$CMD" in \
			*devredis*) \
				kill "$$PID" 2>/dev/null || true; \
				echo "stopped devredis by port scan (pid $$PID)"; \
				stopped=1; \
				;; \
		esac; \
	done; \
	rm -f "$(DEVREDIS_PID)"; \
	if [ "$$stopped" -eq 0 ]; then \
		echo "devredis not running on $$HOST:$$PORT"; \
	fi

.PHONY: dev-redis-status
dev-redis-status: ## Show status of Go-based devredis
	@if [ -f "$(DEVREDIS_PID)" ] && kill -0 "$$(cat "$(DEVREDIS_PID)")" 2>/dev/null; then \
		echo "devredis running (pid $$(cat "$(DEVREDIS_PID)")) on $(REDIS_ADDR)"; \
	else \
		echo "devredis not running"; \
	fi

.PHONY: dev-server
dev-server: dev-prepare ## Run local Go API server
	$(GO) run ./cmd/server

.PHONY: dev-server-air
dev-server-air: dev-server-air-run ## Backward-compatible alias for run profile

.PHONY: dev-server-air-run
dev-server-air-run: dev-tools-install ## Run local API with hot-reload (run profile)
	"$(AIR_BIN)" -c .air.run.toml

.PHONY: dev-server-air-debug
dev-server-air-debug: dev-tools-install ## Run local API with hot-reload + debugger
	@$(MAKE) dev-debugger-down
	"$(AIR_BIN)" -c .air.debug.toml

.PHONY: dev-asynqmon
dev-asynqmon: ## Run Asynq monitor UI (http://localhost:$(ASYNQMON_PORT))
	$(GO) -C $(TOOLS_DIR) run github.com/hibiken/asynqmon/cmd/asynqmon --redis-addr="$(REDIS_ADDR)" --port="$(ASYNQMON_PORT)"

.PHONY: dev-asynqmon-up
dev-asynqmon-up: ## Start Asynq monitor UI in background
	@mkdir -p tmp
	@if [ -f "$(ASYNQMON_PID)" ] && kill -0 "$$(cat "$(ASYNQMON_PID)")" 2>/dev/null; then \
		echo "asynqmon already running (pid $$(cat "$(ASYNQMON_PID)"))"; \
		exit 0; \
	fi
	nohup $(GO) -C $(TOOLS_DIR) run github.com/hibiken/asynqmon/cmd/asynqmon --redis-addr="$(REDIS_ADDR)" --port="$(ASYNQMON_PORT)" >"$(ASYNQMON_LOG)" 2>&1 & echo $$! > "$(ASYNQMON_PID)"
	@sleep 1
	@if ! kill -0 "$$(cat "$(ASYNQMON_PID)")" 2>/dev/null; then \
		echo "failed to start asynqmon, see $(ASYNQMON_LOG)"; \
		rm -f "$(ASYNQMON_PID)"; \
		exit 1; \
	fi
	@echo "asynqmon starting on http://localhost:$(ASYNQMON_PORT) (pid $$(cat "$(ASYNQMON_PID)"))"
	@echo "logs: $(ASYNQMON_LOG)"

.PHONY: dev-asynqmon-down
dev-asynqmon-down: ## Stop background Asynq monitor started by Makefile
	@if [ ! -f "$(ASYNQMON_PID)" ]; then \
		echo "asynqmon not running (no pid file)"; \
		exit 0; \
	fi
	@PID="$$(cat "$(ASYNQMON_PID)")"; \
	if kill -0 "$$PID" 2>/dev/null; then \
		kill "$$PID"; \
		echo "stopped asynqmon (pid $$PID)"; \
	else \
		echo "stale pid file found for asynqmon"; \
	fi
	@rm -f "$(ASYNQMON_PID)"

.PHONY: dev-asynqmon-status
dev-asynqmon-status: ## Show status of background Asynq monitor
	@if [ -f "$(ASYNQMON_PID)" ] && kill -0 "$$(cat "$(ASYNQMON_PID)")" 2>/dev/null; then \
		echo "asynqmon running (pid $$(cat "$(ASYNQMON_PID)")) on http://localhost:$(ASYNQMON_PORT)"; \
	else \
		echo "asynqmon not running"; \
	fi

.PHONY: dev-ui
dev-ui: ## Run Vite dev server for the admin UI (http://localhost:5173); proxies /api to :{SERVER_PORT}
	npm run dev --prefix web

## ------- Codegen / DB -------

.PHONY: tools
tools: ## Install sqlc + goose CLIs
	$(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

.PHONY: sqlc
sqlc: ## Generate typed query code from db/queries
	"$$($(GO) env GOPATH)/bin/sqlc" generate

MIGRATION_NAME ?= new_migration

.PHONY: migrate-new
migrate-new: ## Create a new migration pair: make migrate-new MIGRATION_NAME=add_users
	goose -dir $(DATABASE_MIGRATIONS_DIR) create $(MIGRATION_NAME) sql

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	goose -dir $(DATABASE_MIGRATIONS_DIR) sqlite3 "$(DATABASE_PATH)" up

.PHONY: migrate-down
migrate-down: ## Revert the most recent migration
	goose -dir $(DATABASE_MIGRATIONS_DIR) sqlite3 "$(DATABASE_PATH)" down

.PHONY: migrate-status
migrate-status: ## Show the current migration version
	goose -dir $(DATABASE_MIGRATIONS_DIR) sqlite3 "$(DATABASE_PATH)" status

## ------- Docker (Container Workflow Only) -------

.PHONY: docker-build
docker-build: ## Build the Docker image
	$(DOCKER) build -t $(APP_NAME):latest .

.PHONY: up
up: ## docker compose up (build + detach)
	$(COMPOSE) up --build -d

.PHONY: down
down: ## docker compose down
	$(COMPOSE) down

.PHONY: logs
logs: ## Tail compose logs
	$(COMPOSE) logs -f --tail=200

.PHONY: clean
clean: ## Remove local build artifacts
	rm -rf $(BIN_DIR) tmp
