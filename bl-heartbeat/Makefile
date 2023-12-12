GITCOMMIT ?= $(shell git rev-parse HEAD)
GITDATE ?= $(shell git show -s --format='%ct')
VERSION := v0.0.0

LDFLAGSSTRING +=-X main.GitCommit=$(GITCOMMIT)
LDFLAGSSTRING +=-X main.GitDate=$(GITDATE)
LDFLAGSSTRING +=-X main.Version=$(VERSION)
LDFLAGS := -ldflags "$(LDFLAGSSTRING)"

bl-heartbeat:
	env GO111MODULE=on go build -v $(LDFLAGS) -o ./bin/bl-heartbeat ./cmd

clean:
	rm bin/bl-heartbeat

test:
	go test -v ./...

.PHONY: \
	clean \
	bl-heartbeat \
	test \
	lint
