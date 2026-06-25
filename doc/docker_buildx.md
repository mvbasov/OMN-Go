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
