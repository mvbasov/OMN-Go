rm -rf ./output-binaryes
docker create --name goomn-extract goomn-builder
docker cp goomn-extract:/app/bin/ ./output-binaries/
docker rm goomn-extract

