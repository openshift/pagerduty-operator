#!/bin/bash

# AppSRE team CD

set -exv

_OPERATOR_NAME="pagerduty-operator"

CURRENT_DIR=$(dirname "$0")

BASE_IMG="${_OPERATOR_NAME}"
QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"
IMG="${BASE_IMG}:latest"

GIT_HASH=$(git rev-parse --short=7 HEAD)

if [[ -z $IMAGE_REPOSITORY ]]; then
  IMAGE_REPOSITORY=app-sre
fi

# build and push the operator and catalog images
make IMAGE_REPOSITORY=$IMAGE_REPOSITORY build skopeo-push build-catalog-image
