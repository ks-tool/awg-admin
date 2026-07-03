GOOS   ?= linux
GOARCH ?= amd64

BUILD_DIR := ../build/bin/

GO_LD_FLAGS    := '-s -w'
GO_GC_FLAGS    := '-N -l'
GO_BUILD_FLAGS := -ldflags=$(GO_LD_FLAGS) -gcflags=$(GO_GC_FLAGS) -trimpath -o $(BUILD_DIR)

.PHONY: build
build:
	@echo 'Build agent (kernel / amneziawg-dkms) ...'
	@GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) cmd/awg-agent.go

.PHONY: build-userspace
build-userspace:
	@echo 'Build agent (userspace / amneziawg-go) ...'
	@GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) cmd/awg-agent-userspace.go

# Unit tests for the agent module. The e2e module (agent/e2e) is a separate
# module, so `./...` skips it — run it with `make test-e2e`.
.PHONY: test
test:
	@echo 'Test agent module ...'
	@GOEXPERIMENT=jsonv2 go test ./...

# End-to-end tunnel test (agent/e2e): builds the userspace image from source and
# runs two containers that form a real AmneziaWG tunnel. Needs Docker with
# /dev/net/tun. It's a separate module (keeps testcontainers out of the daemon
# module); GOWORK=off is a defensive no-op now the repo has no go.work, but keeps
# the e2e module isolated even if someone adds one back locally.
.PHONY: test-e2e
test-e2e:
	@echo 'Run userspace tunnel e2e (needs Docker) ...'
	@cd e2e && GOWORK=off AWG_E2E=1 go test -run TestUserspaceTunnel -v -timeout 600s .
