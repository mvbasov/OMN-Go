#!/bin/sh
# android/fdroid_prepare_aar.sh
#
# NOT USED by the current F-Droid build recipe
# (metadata/net.basov.omngo.fdroid.yml). Kept only as a leftover from an
# earlier design; see below before reviving or deleting it.
#
# ORIGINAL PLAN (superseded): this script was written to copy a
# PREBUILT omngo.aar into app/libs/ before Gradle runs, sourced from a
# separate Go-backend repository via F-Droid's "srclib" mechanism (see
# git history of this file for the original header, which spelled out
# the srclibs:/$$GoBackend$$ wiring in detail).
#
# WHAT ACTUALLY HAPPENS NOW: that separate repository was never created.
# metadata/net.basov.omngo.fdroid.yml's prebuild: step instead
# bootstraps a Go toolchain and runs `gomobile bind` directly, in-tree,
# producing android/app/libs/omngo.aar itself - no srclib, no separate
# repo, and no call into this script. If that ever changes back to a
# srclib-based split build, this script (or something like it) would
# need to be wired back into that recipe's prebuild: - until then it is
# dead code, left in place only in case that direction is revisited.
