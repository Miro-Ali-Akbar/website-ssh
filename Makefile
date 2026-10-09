# Everything runs in containers, so only podman is needed.
IMAGE ?= website-ssh
PORT  ?= 2222
# Counters are off by default so testing doesn't bump the real ones.
# Use the live ones with: make run COUNTERS_URL=https://miro.zip/api
COUNTERS_URL ?=
GO    := podman run --rm -v "$(CURDIR)":/src:Z -v website-ssh-gocache:/go -w /src docker.io/library/golang:1.26

.PHONY: help build run run-local ssh check snapshot clean

help: ## Show this list
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

build: ## Build the container image
	podman build -t $(IMAGE) .

run: build ## Run the server, reading content from the live miro.zip
	podman run --rm -p $(PORT):2222 -e COUNTERS_URL=$(COUNTERS_URL) $(IMAGE)

run-local: build ## Run the server, reading content from ../website/serve.sh on port 8000
	podman run --rm --network host -e LISTEN_ADDR=:$(PORT) -e SITE_URL=http://127.0.0.1:8000 -e COUNTERS_URL=$(COUNTERS_URL) $(IMAGE)

ssh: ## Connect to the running server (host key is temporary, so known_hosts is skipped)
	ssh -p $(PORT) -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost

check: ## Format check and go vet
	@test -z "$$($(GO) gofmt -l .)" || { echo "Not gofmt'd:"; $(GO) gofmt -l .; exit 1; }
	$(GO) go vet ./...

snapshot: ## Copy the website's HTML into snapshot/ (fallback content)
	cp ../website/index.html ../website/projects.html snapshot/

clean: ## Remove the image and local binary
	-podman rmi $(IMAGE)
	rm -f website-ssh
