GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

BUILD_DIR := build/bin/

GO_LD_FLAGS    := '-s -w'
GO_GC_FLAGS    := '-N -l'
GO_BUILD_FLAGS := -ldflags=$(GO_LD_FLAGS) -gcflags=$(GO_GC_FLAGS) -trimpath -o $(BUILD_DIR)

.PHONY: server
server: frontend
	@echo 'Build server ...'
	@GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) cmd/awg-admin.go

.PHONY: run-server
run-server: frontend
	@GOEXPERIMENT=jsonv2 go run cmd/awg-admin.go

.PHONY: desktop
desktop:
	@echo 'Build desktop application ...'
	@GOEXPERIMENT=jsonv2 go tool wails build -race -platform=$(GOOS)/$(GOARCH)

.PHONY: agent
agent:
	@make -C agent build

.PHONY: migrate
migrate:
	@echo 'Build awg-migrate ...'
	@GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) ./cmd/migrate

.PHONY: frontend
frontend:
	@echo 'Build UI ...'
	@cd frontend && npm run build