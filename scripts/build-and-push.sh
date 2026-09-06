#!/usr/bin/env bash

set -Eeuo pipefail

: "${REGISTRY_IMAGE:?Set REGISTRY_IMAGE to the image repository, for example registry.example.com/team/autoscaler}"

IMAGE_TAG="${IMAGE_TAG:-next}"
PLATFORMS="${PLATFORMS:-linux/amd64}"
DOCKERFILE="${DOCKERFILE:-docker/Dockerfile}"
CONTEXT="${CONTEXT:-.}"

command -v docker >/dev/null 2>&1 || {
	echo "docker is required" >&2
	exit 1
}

docker buildx version >/dev/null 2>&1 || {
	echo "docker buildx is required" >&2
	exit 1
}

image="${REGISTRY_IMAGE}:${IMAGE_TAG}"
echo "Building and pushing ${image} for ${PLATFORMS}"

docker buildx build \
	--platform "${PLATFORMS}" \
	--file "${DOCKERFILE}" \
	--tag "${image}" \
	--push \
	"${CONTEXT}"
