FROM --platform=$BUILDPLATFORM golang:1.21.3-alpine3.18 as builder

RUN apk add --no-cache make gcc musl-dev linux-headers git jq bash

# We copy the go.mod/sum first, so the `go mod download` does not have to re-run if dependencies do not change.
COPY ./go.mod /app/go.mod
COPY ./go.sum /app/go.sum

WORKDIR /app

RUN echo "go mod cache: $(go env GOMODCACHE)"
RUN echo "go build cache: $(go env GOCACHE)"

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download

# NOTE: the Dockerfile.dockerignore file effectively describes all dependencies
COPY . /app

# We avoid copying the full .git dir into the build for just some metadata.
# Instead, specify:
# --build-arg GIT_COMMIT=$(git rev-parse HEAD)
# --build-arg GIT_DATE=$(git show -s --format='%ct')
ARG GIT_COMMIT
ARG GIT_DATE

ARG CANNON_VERSION=v0.0.0
ARG OP_PROGRAM_VERSION=v0.0.0

ARG OP_HEARTBEAT_VERSION=v0.0.0
ARG OP_WHEEL_VERSION=v0.0.0

ARG OP_NODE_VERSION=v0.0.0
ARG OP_CHALLENGER_VERSION=v0.0.0
ARG OP_BATCHER_VERSION=v0.0.0
ARG OP_PROPOSER_VERSION=v0.0.0


# separate docker-builds:
# - op-exporter
# - op-ufm
# - proxyd
# - any JS/TS/smart-contract builds

ARG TARGETOS TARGETARCH

# Build the Go services, utilizing caches and share the many common packages.
# The "id" defaults to the value of "target", the cache will thus be reused during this build.
# "sharing" defaults to "shared", the cache will thus be available to other concurrent docker builds.

RUN --mount=type=cache,target=/root/.cache/go-build cd cannon && make cannon  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE VERSION="$CANNON_VERSION"
# note: we only build the host, that's all the user needs. No Go MIPS cross-build in docker
RUN --mount=type=cache,target=/root/.cache/go-build cd bl-program && make bl-program-host  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE VERSION="$OP_PROGRAM_VERSION"

RUN --mount=type=cache,target=/root/.cache/go-build cd bl-heartbeat && make bl-heartbeat  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE VERSION="$OP_HEARTBEAT_VERSION"
RUN --mount=type=cache,target=/root/.cache/go-build cd bl-wheel && make bl-wheel  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE  VERSION="$OP_WHEEL_VERSION"

RUN --mount=type=cache,target=/root/.cache/go-build cd bl-node && make bl-node  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE VERSION="$OP_NODE_VERSION"
RUN --mount=type=cache,target=/root/.cache/go-build cd bl-challenger && make bl-challenger  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE  VERSION="$OP_CHALLENGER_VERSION"
RUN --mount=type=cache,target=/root/.cache/go-build cd bl-batcher && make bl-batcher  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE  VERSION="$OP_BATCHER_VERSION"
RUN --mount=type=cache,target=/root/.cache/go-build cd bl-proposer && make bl-proposer  \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GITCOMMIT=$GIT_COMMIT GITDATE=$GIT_DATE  VERSION="$OP_PROPOSER_VERSION"


FROM alpine:3.18

COPY --from=builder /app/cannon/bin/cannon /usr/local/bin/
COPY --from=builder /app/bl-program/bin/bl-program /usr/local/bin/

COPY --from=builder /app/bl-heartbeat/bin/bl-heartbeat /usr/local/bin/
COPY --from=builder /app/bl-wheel/bin/bl-wheel /usr/local/bin/

COPY --from=builder /app/bl-node/bin/bl-node /usr/local/bin/
COPY --from=builder /app/bl-challenger/bin/bl-challenger /usr/local/bin/
COPY --from=builder /app/bl-batcher/bin/bl-batcher /usr/local/bin/
COPY --from=builder /app/bl-proposer/bin/bl-proposer /usr/local/bin/


