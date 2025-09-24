# --- Configuration ---
# OS variant (e.g., ubuntu24.04, rockylinux9)
OS ?= ubuntu24.04
# Slurm version, also used for the directory path
SLURM_VERSION ?= 24.11

# Container registry and repository
REGISTRY ?= us-west1-docker.pkg.dev/kevinmcw-gke-dev # Add your Google Artifact Registry here like `us-east1-docker.pkg.dev/my-repo`
REPO ?= kevinmcw

# Build arguments for Pyxis
ENROOT_VERSION ?= 3.5.0
PYXIS_VERSION ?= 0.20.0


# --- Path and Image Naming ---
# Path to the build context directories
BUILD_PATH := slurm/$(SLURM_VERSION)

# Image tag
TAG ?= $(SLURM_VERSION)-$(OS)

# Defines the full names for target images
SLURMD_IMAGE         := $(REGISTRY)/$(REPO)/slurmd:$(TAG)
SLURMD_PYXIS_IMAGE   := $(REGISTRY)/$(REPO)/slurmd-pyxis:$(TAG)


# --- Targets ---
.PHONY: all build push clean
.PHONY: build-slurmd build-slurmd-pyxis
.PHONY: push-slurmd push-slurmd-pyxis

all: build

build: build-slurmd build-slurmd-pyxis
	@echo "All images built successfully."

push: push-slurmd push-slurmd-pyxis
	@echo "All images pushed successfully."


# --- Individual Build Targets ---
build-slurmd:
	@echo "Building $(SLURMD_IMAGE) with buildx..."
	docker buildx build \
		--file $(BUILD_PATH)/$(OS)/Dockerfile \
		--target slurmd \
		--progress=plain \
		--tag $(SLURMD_IMAGE) \
		$(BUILD_PATH)/$(OS)/

build-slurmd-pyxis: build-slurmd
	@echo "Building $(SLURMD_PYXIS_IMAGE) with buildx..."
	docker buildx build \
		--file $(BUILD_PATH)/$(OS)/Dockerfile.pyxis \
		--target slurmd-pyxis \
		--build-arg ENROOT_VERSION=$(ENROOT_VERSION) \
		--build-arg PYXIS_VERSION=$(PYXIS_VERSION) \
		--build-arg BASE_CONTAINER_PATH=$(SLURMD_IMAGE) \
		--tag $(SLURMD_PYXIS_IMAGE) \
		$(BUILD_PATH)/$(OS)/

# --- Individual Push Targets ---
push-slurmd:
	@echo "Pushing $(SLURMD_IMAGE)..."
	docker push $(SLURMD_IMAGE)

push-slurmd-pyxis:
	@echo "Pushing $(SLURMD_PYXIS_IMAGE)..."
	docker push $(SLURMD_PYXIS_IMAGE)


# --- Housekeeping ---
clean:
	@echo "Cleaning up local images..."
	-docker rmi $(SLURMD_IMAGE) || true
	-docker rmi $(SLURMD_PYXIS_IMAGE) || true