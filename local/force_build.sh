#!/bin/bash
# The local build without the build cache. See local/build.sh.
exec bash "$(dirname "$0")/build.sh" --no-cache "$@"
