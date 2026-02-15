#!/bin/bash
# Build and push the Dashboard Docker image to the local registry
# Usage: ./build-dashboard.sh [tag]

set -euo pipefail

# Configuration
REGISTRY="registry.petersimmons.com"
IMAGE_NAME="job-tracker-dashboard"
DEFAULT_TAG="latest"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse arguments
TAG="${1:-$DEFAULT_TAG}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "=========================================="
echo "Building Job Tracker Dashboard"
echo "=========================================="
echo "Registry:    ${REGISTRY}"
echo "Image:       ${IMAGE_NAME}"
echo "Tag:         ${TAG}"
echo "Full image:  ${FULL_IMAGE}"
echo "=========================================="

# Verify registry is accessible
echo "Checking registry connectivity..."
if ! curl -sf -o /dev/null "https://${REGISTRY}/v2/"; then
    echo "ERROR: Cannot reach registry at ${REGISTRY}"
    echo "Verify the registry is running: curl -I https://${REGISTRY}/v2/"
    exit 1
fi
echo "Registry is accessible."

# Build the Docker image
echo ""
echo "Building Docker image..."
cd "${SCRIPT_DIR}"
docker build \
    -f Dockerfile.dashboard \
    -t "${IMAGE_NAME}:${TAG}" \
    -t "${FULL_IMAGE}" \
    .

echo ""
echo "Build complete."

# Push to registry
echo ""
echo "Pushing to registry..."
docker push "${FULL_IMAGE}"

# Also push latest if we're building a versioned tag
if [[ "${TAG}" != "latest" ]]; then
    LATEST_IMAGE="${REGISTRY}/${IMAGE_NAME}:latest"
    echo "Also tagging and pushing as latest..."
    docker tag "${FULL_IMAGE}" "${LATEST_IMAGE}"
    docker push "${LATEST_IMAGE}"
fi

echo ""
echo "=========================================="
echo "SUCCESS: Image pushed to registry"
echo "=========================================="
echo "To deploy, run:"
echo "  kubectl apply -f ~/projects/job-search-system/k8s/dashboard-configmap.yaml"
echo "  kubectl apply -f ~/projects/job-search-system/k8s/dashboard-deployment.yaml"
echo "  kubectl apply -f ~/projects/job-search-system/k8s/dashboard-service.yaml"
echo "  kubectl apply -f ~/projects/job-search-system/k8s/dashboard-ingress.yaml"
echo ""
echo "Or apply all at once:"
echo "  kubectl apply -f ~/projects/job-search-system/k8s/dashboard-*.yaml"
echo ""
echo "To check deployment status:"
echo "  kubectl get pods -n job-search -l app=dashboard"
echo "  kubectl logs -n job-search -l app=dashboard --tail=100"
echo "=========================================="
