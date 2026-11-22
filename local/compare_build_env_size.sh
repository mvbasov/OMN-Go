#!/bin/sh
# Compare the base_env image from a Git revision with the local image build.
# Usage: sh local/compare_build_env_size.sh [baseline-ref]
set -eu
cd "$(dirname "$0")/.."

# This commit uses the Debian build image.
baseline=$(git rev-parse --verify --end-of-options "${1:-68e2bf44d3496ee043f789e63746cbbd9abdce4a}^{commit}")
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

git archive --format=tar --output="$tmp_dir/baseline.tar" "$baseline"

docker buildx build --platform linux/amd64 --target base_env --load \
    --iidfile "$tmp_dir/baseline.iid" - < "$tmp_dir/baseline.tar"
docker buildx build --platform linux/amd64 --target base_env --load \
    --iidfile "$tmp_dir/current.iid" .

baseline_size=$(docker image inspect --format '{{.Size}}' "$(cat "$tmp_dir/baseline.iid")")
current_size=$(docker image inspect --format '{{.Size}}' "$(cat "$tmp_dir/current.iid")")

LC_ALL=C awk -v baseline="$baseline" -v before="$baseline_size" -v after="$current_size" '
    BEGIN {
        mib = 1024 * 1024
        printf "Baseline %s: %.0f bytes (%.1f MiB)\n", baseline, before, before / mib
        printf "Current checkout: %.0f bytes (%.1f MiB)\n", after, after / mib
        printf "Change: %+.0f bytes (%+.1f MiB, %+.1f%%)\n",
            after - before, (after - before) / mib, 100 * (after - before) / before
    }
'
