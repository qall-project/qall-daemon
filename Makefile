.PHONY: help clean-containers clean-all build-daemon build reset

# Variables
IMAGE_DAEMON := scw/qall-daemon-server
TAG_DAEMON := latest

DOCKERFILE_DAEMON := docker/qall-daemon.Dockerfile


# Default target
help:
	@echo "Available commands:"
	@echo "  make clean-containers  - Stop qall-tasks, stop qall-workers, prune containers, remove qall-task images"
	@echo "  make clean-all      - Stop ALL containers, prune, remove base & daemon images"
	@echo "  make build          - Build daemon image. Alias of build-daemon"
	@echo "  make reset          - Full reset: clean-all followed by build"

# Regular Clean
clean-containers:
	@echo "Stopping qall-task containers..."
	@containers=$$(docker ps -q --filter "name=qall-task"); \
	if [ -n "$$containers" ]; then docker stop $$containers; else echo "  -> No task running."; fi

	@containers=$$(docker ps -q --filter "name=qall-worker"); \
	if [ -n "$$containers" ]; then docker stop $$containers; else echo "  -> No worker running."; fi

	@echo "Pruning stopped containers..."
	@docker container prune -f

	@echo "Removing qall-task images..."
	@images=$$(docker images -q "qall-task"); \
	if [ -n "$$images" ]; then docker image rm -f $$images; else echo "  -> No qall-task images found."; fi

	@echo "Removing qall-worker images..."
	@images=$$(docker images -q "qall-worker"); \
	if [ -n "$$images" ]; then docker image rm -f $$images; else echo "  -> No qall-worker images found."; fi

# Full Reset Clean
clean-all: clean-containers
	@echo "Stopping ALL containers..."
	@containers=$$(docker ps -q); \
	if [ -n "$$containers" ]; then docker stop $$containers; else echo "  -> No containers running."; fi
	@echo "Pruning stopped containers..."
	@docker container prune -f
# 	Uncomment to clean qall-base image too
	if [ -n "$$images" ]; then docker image rm -f $$images; else echo "  -> No base images found."; fi
	@echo "Removing $(IMAGE_DAEMON) images..."
	@images=$$(docker images -q "$(IMAGE_DAEMON)"); \
	if [ -n "$$images" ]; then docker image rm -f $$images; else echo "  -> No daemon images found."; fi

build-daemon:
	@echo "Building $(IMAGE_DAEMON):$(TAG_DAEMON)..."
	docker build --no-cache -t $(IMAGE_DAEMON):$(TAG_DAEMON) -f $(DOCKERFILE_DAEMON) .

build: build-daemon
	docker images

# Full Reset
reset: clean-all build
