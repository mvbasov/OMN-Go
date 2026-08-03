#!/bin/bash
if [ ! -f android/app/omn-go.keystore ]; then \
   keytool -genkey -v -keystore app/omn-go.keystore \
           -alias omn-go -keyalg RSA -keysize 2048 \
           -validity 10000 -storepass omn-go123 -keypass omn-go123 \
           -dname "CN=OMN-Go, O=Basov"; \
fi 

echo "-------- STAGE 1 -------"
docker buildx build -f Dockerfile.base -t omn-go-base:latest .
echo "-------- STAGE 2 -------"
docker buildx build -t omn-go-builder:latest . \
   $(grep -v '^#' .env | xargs -I {} echo --build-arg {})

rm -rf ./output-binaries/

mkdir -p ./output-binaries/

docker create --name omn-go-extract omn-go-builder

# The /. suffix copies the contents of bin, not the directory itself.
docker cp omn-go-extract:/app/bin/. ./output-binaries/

docker rm omn-go-extract

ls -l ./output-binaries/
echo "Binaries successfully extracted to perfectly clean ./output-binaries/ directory!"
