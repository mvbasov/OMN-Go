#!/bin/sh
# Measure the size of the desktop and Android Go binaries against a base
# version. Usage: sh local/compare_size.sh [base ref]
#
# local/Dockerfile.compare starts from omn-go-base:latest. The first build
# below tags the base_env stage of the Dockerfile with that name. The
# build cache makes it short after a local/build.sh run.
set -eu
cd "$(dirname "$0")/.."
docker buildx build --target base_env --load -t omn-go-base:latest .
docker buildx build -f local/Dockerfile.compare --progress=plain --build-arg RUN_ID=$(date +%s) --build-arg SIZE_BASE=${1:-} --output type=cacheonly .
