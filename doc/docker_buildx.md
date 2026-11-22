If docker buildx missing
```
mkdir -p ~/.docker/cli-plugins
HOST="https://github.com"
PATH_NAME="/docker/buildx/releases/download/v0.14.0"
FILE_NAME="buildx-v0.14.0.linux-amd64"
curl -SL "${HOST}${PATH_NAME}/${FILE_NAME}" -o ~/.docker/cli-plugins/docker-buildx
chmod +x ~/.docker/cli-plugins/docker-buildx
docker buildx version
```

## Compare build environment sizes

Run this command from the repository root.

```sh
sh local/compare_build_env_size.sh
```

The script builds `base_env` for the last Debian revision and for your current
checkout. It uses `linux/amd64` for both builds. Docker stores both images
locally. The script reports their sizes in bytes and MiB, and the percentage
change. The sizes exclude the BuildKit cache and the release artifacts.
The script needs no signing keys.

To use a different baseline, pass a Git revision.

```sh
sh local/compare_build_env_size.sh <baseline-ref>
```
