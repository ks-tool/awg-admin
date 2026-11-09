GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

BUILD_DIR := build/bin/

GO_LD_FLAGS    := '-s -w'
GO_GC_FLAGS    := '-N -l'
GO_BUILD_FLAGS := -ldflags=$(GO_LD_FLAGS) -gcflags=$(GO_GC_FLAGS) -trimpath -o $(BUILD_DIR)

.PHONY: server
server: frontend
	@echo 'Build server ...'
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) cmd/vpn-admin.go

.PHONY: run-server
run-server: frontend
	@go run cmd/vpn-admin.go

.PHONY: desktop
desktop:
	@echo 'Build desktop application ...'
	@go tool wails build -race -platform=$(GOOS)/$(GOARCH)

.PHONY: agent
agent:
	@make -C agent build

.PHONY: provider-awg
provider-awg:
	@make -C agent build-provider-awg

.PHONY: migrate
migrate:
	@echo 'Build awg-migrate ...'
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) ./cmd/migrate

.PHONY: frontend
frontend:
	@echo 'Build UI ...'
	@cd frontend && npm run build

# Unit tests for both Go modules. `go test ./...` at the root only covers the
# root module (agent is a separate module — its own go.mod, pulled in as the
# published github.com/ks-tool/vpn-agent), so the agent module is tested
# via its own Makefile; node_modules holds a stray vendored Go package that
# isn't ours, so it's filtered out. The Docker-based e2e tunnel test is
# separate and opt-in — run `make -C agent test-e2e` (needs Docker).
.PHONY: test
test:
	@echo 'Test root module ...'
	@go test $$(go list ./... | grep -v /node_modules/)
	@$(MAKE) -C agent test
