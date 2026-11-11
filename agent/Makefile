GOOS   ?= linux
GOARCH ?= amd64

BUILD_DIR := ../build/bin/

GO_LD_FLAGS    := '-s -w'
GO_GC_FLAGS    := '-N -l'
GO_BUILD_FLAGS := -ldflags=$(GO_LD_FLAGS) -gcflags=$(GO_GC_FLAGS) -trimpath -o $(BUILD_DIR)

.PHONY: build
build:
	@echo 'Build vpn-agent ...'
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS)vpn-agent ./cmd/vpn-agent

.PHONY: build-provider-awg
build-provider-awg:
	@echo 'Build AWG provider (automatic kernel/userspace backend) ...'
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS)vpn-provider-awg ./providers/awg/cmd

.PHONY: build-provider-xray
build-provider-xray:
	@echo 'Build Xray provider ...'
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS)vpn-provider-xray ./providers/xray/cmd

.PHONY: build-provider-hysteria2
build-provider-hysteria2:
	@echo 'Build Hysteria 2 provider ...'
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS)vpn-provider-hysteria2 ./providers/hysteria2/cmd

.PHONY: test
test:
	@echo 'Test agent module ...'
	@go test ./...

# End-to-end tunnel test (agent/e2e): builds the userspace AWG provider image
# from source and runs two containers that form a real AmneziaWG tunnel.
.PHONY: test-e2e
test-e2e:
	@echo 'Run userspace tunnel e2e (needs Docker) ...'
	@cd e2e && GOWORK=off AWG_E2E=1 go test -run TestUserspaceTunnel -v -timeout 600s .
