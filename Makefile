BIN_DIR=$(CURDIR)/bin

GOLINT_BIN_DIR=$(CURDIR)/bin
GOLINT_CMD=$(GOLINT_BIN_DIR)/golangci-lint
GOLINT_VERSION=v2.11.4

DOCKER_IMAGE ?= iris

.PHONY: lint test build run clean docker/build docker/run docker/start docker/stop docker/logs

lint:
	@echo "Linting Go code..."
	@if ! command -v $(GOLINT_CMD) &> /dev/null; then \
		echo "golangci-lint $(GOLINT_VERSION) not found or wrong version, installing to $(GOLINT_BIN_DIR)..."; \
		curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOLINT_BIN_DIR) $(GOLINT_VERSION); \
	fi
	$(GOLINT_CMD) run ./...

test:
	go test ./... -v -timeout 60s

build:
	go build -o $(BIN_DIR)/iris ./cmd/iris

run: build
	$(BIN_DIR)/iris

docker/build:
	docker build -t $(DOCKER_IMAGE) .

docker/run: docker/build
	docker run --rm \
		-p 3000:3000 \
		--add-host=host.docker.internal:host-gateway \
		-e IRIS_ADDR=0.0.0.0:3000 \
		$(DOCKER_IMAGE)

docker/start: docker/build
	docker run -d --restart=unless-stopped \
		--name $(DOCKER_IMAGE) \
		-p 3000:3000 \
		--add-host=host.docker.internal:host-gateway \
		-e IRIS_ADDR=0.0.0.0:3000 \
		$(DOCKER_IMAGE)

docker/stop:
	docker stop $(DOCKER_IMAGE) && docker rm $(DOCKER_IMAGE)

docker/logs:
	docker logs -f $(DOCKER_IMAGE)

clean:
	rm -f $(BIN_DIR)/iris