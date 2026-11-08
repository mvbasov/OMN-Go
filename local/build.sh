#!/bin/bash
# local/build.sh - the local build of the desktop binaries and of the APKs.
#
# It builds the same Dockerfile, the same stages and the same secret ids as
# GitHub and GitLab. The artifacts go to ./output-binaries/.
#
# USAGE (from any directory):
#   bash local/build.sh               # the normal build
#   bash local/build.sh --no-cache    # each other argument goes to "docker buildx build"
#
# THE SIGNING MATERIAL.
#   android/app/omn-go.keystore   The keystore. The script makes one when it is absent.
#   .env                          KEYSTORE_PASSWORD, KEY_ALIAS and KEY_PASSWORD,
#                                 one NAME=value line each. Optional.
# Git ignores both files, and .dockerignore keeps both out of the build context.
# They go to the build only as BuildKit secrets.

set -euo pipefail

cd "$(dirname "$0")/.."

KEYSTORE=android/app/omn-go.keystore

# Read only the three names from .env. The file is not run as a script.
if [ -f .env ]; then
    while IFS='=' read -r name value; do
        case "$name" in
            KEYSTORE_PASSWORD|KEY_ALIAS|KEY_PASSWORD) export "$name=$value" ;;
        esac
    done < .env
fi
export KEYSTORE_PASSWORD="${KEYSTORE_PASSWORD:-omn-go123}"
export KEY_ALIAS="${KEY_ALIAS:-omn-go}"
export KEY_PASSWORD="${KEY_PASSWORD:-omn-go123}"

if [ ! -f "$KEYSTORE" ]; then
    echo "-------- make $KEYSTORE --------"
    keytool -genkey -v -keystore "$KEYSTORE" \
            -alias "$KEY_ALIAS" -keyalg RSA -keysize 2048 \
            -validity 10000 -storepass "$KEYSTORE_PASSWORD" -keypass "$KEY_PASSWORD" \
            -dname "CN=OMN-Go, O=Basov"
fi

rm -rf ./output-binaries/

echo "-------- build --------"
docker buildx build \
    --target export \
    --output type=local,dest=output-binaries \
    --secret id=keystore,src="$KEYSTORE" \
    --secret id=keystore_password,env=KEYSTORE_PASSWORD \
    --secret id=key_alias,env=KEY_ALIAS \
    --secret id=key_password,env=KEY_PASSWORD \
    "$@" \
    .

ls -l ./output-binaries/
echo "The artifacts are in ./output-binaries/."
