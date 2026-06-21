GOARCH ?= amd64

BUILD_DIR := ../build/bin/

GO_LD_FLAGS    := '-s -w'
GO_GC_FLAGS    := '-N -l'
GO_BUILD_FLAGS := -ldflags=$(GO_LD_FLAGS) -gcflags=$(GO_GC_FLAGS) -trimpath -o $(BUILD_DIR)

.PHONY: build
build:
	@echo 'Build agent ...'
	@GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build $(GO_BUILD_FLAGS) cmd/awg-agent.go
