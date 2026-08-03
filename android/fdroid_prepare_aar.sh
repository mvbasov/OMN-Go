#!/bin/sh
# The F-Droid build must be reproducible and cannot fetch what the normal
# build fetches. This script copies a prebuilt omngo.aar from an F-Droid
# srclib into app/libs/ to meet that constraint.
#
# TODO: dead code. metadata/net.basov.omngo.fdroid.yml never calls it and
# runs `gomobile bind` in-tree instead. Delete it or wire it in.
