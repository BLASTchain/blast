variable "REGISTRY" {
  default = "us-docker.pkg.dev"
}

variable "REPOSITORY" {
  default = "oplabs-tools-artifacts/images"
}

variable "GIT_COMMIT" {
  default = "dev"
}

variable "GIT_DATE" {
  default = "0"
}

variable "GIT_VERSION" {
  default = "docker"  // original default as set in proxyd file, not used by full go stack, yet
}

variable "IMAGE_TAGS" {
  default = "${GIT_COMMIT}" // split by ","
}

variable "PLATFORMS" {
  // You can override this as "linux/amd64,linux/arm64".
  // Only a specify a single platform when `--load` ing into docker.
  // Multi-platform is supported when outputting to disk or pushing to a registry.
  // Multi-platform builds can be tested locally with:  --set="*.output=type=image,push=false"
  default = "linux/amd64"
}

target "op-stack-go" {
  dockerfile = "ops/docker/op-stack-go/Dockerfile"
  context = "."
  args = {
    GIT_COMMIT = "${GIT_COMMIT}"
    GIT_DATE = "${GIT_DATE}"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/op-stack-go:${tag}"]
}

target "bl-node" {
  dockerfile = "Dockerfile"
  context = "./bl-node"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-node:${tag}"]
}

target "bl-batcher" {
  dockerfile = "Dockerfile"
  context = "./bl-batcher"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-batcher:${tag}"]
}

target "bl-proposer" {
  dockerfile = "Dockerfile"
  context = "./bl-proposer"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-proposer:${tag}"]
}

target "bl-challenger" {
  dockerfile = "Dockerfile"
  context = "./bl-challenger"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-challenger:${tag}"]
}

target "bl-heartbeat" {
  dockerfile = "Dockerfile"
  context = "./bl-heartbeat"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-heartbeat:${tag}"]
}

target "bl-program" {
  dockerfile = "Dockerfile"
  context = "./bl-program"
  args = {
    OP_STACK_GO_BUILDER = "op-stack-go"
  }
  contexts = {
    op-stack-go: "target:op-stack-go"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/bl-program:${tag}"]
}

target "proxyd" {
  dockerfile = "Dockerfile"
  context = "./proxyd"
  args = {
    // proxyd dockerfile has no _ in the args
    GITCOMMIT = "${GIT_COMMIT}"
    GITDATE = "${GIT_DATE}"
    GITVERSION = "${GIT_VERSION}"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/proxyd:${tag}"]
}

target "indexer" {
  dockerfile = "./indexer/Dockerfile"
  context = "./"
  args = {
    // proxyd dockerfile has no _ in the args
    GITCOMMIT = "${GIT_COMMIT}"
    GITDATE = "${GIT_DATE}"
    GITVERSION = "${GIT_VERSION}"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/indexer:${tag}"]
}

target "ufm-metamask" {
  dockerfile = "Dockerfile"
  context = "./ufm-test-services/metamask"
  args = {
    // proxyd dockerfile has no _ in the args
    GITCOMMIT = "${GIT_COMMIT}"
    GITDATE = "${GIT_DATE}"
    GITVERSION = "${GIT_VERSION}"
  }
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/ufm-metamask:${tag}"]
}

target "chain-mon" {
  dockerfile = "./ops/docker/Dockerfile.packages"
  context = "."
  args = {
    // proxyd dockerfile has no _ in the args
    GITCOMMIT = "${GIT_COMMIT}"
    GITDATE = "${GIT_DATE}"
    GITVERSION = "${GIT_VERSION}"
  }
  // this is a multi-stage build, where each stage is a possible output target, but wd-mon is all we publish
  target = "wd-mon"
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/chain-mon:${tag}"]
}

target "ci-builder" {
  dockerfile = "./ops/docker/ci-builder/Dockerfile"
  context = "."
  platforms = split(",", PLATFORMS)
  tags = [for tag in split(",", IMAGE_TAGS) : "${REGISTRY}/${REPOSITORY}/ci-builder:${tag}"]
}


