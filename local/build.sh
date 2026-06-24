#!/bin/bash
if [ ! -f android/app/omn-go.keystore ]; then \
   keytool -genkey -v -keystore app/omn-go.keystore \
           -alias omn-go -keyalg RSA -keysize 2048 \
           -validity 10000 -storepass omn-go123 -keypass omn-go123 \
           -dname "CN=OMN-Go, O=Basov"; \
fi 

# 0. Build descktop and android binary
docker build -t omn-go-builder .

# 1. Clean up the old directory 
rm -rf ./output-binaries/

# 2. Recreate a fresh, empty directory
mkdir -p ./output-binaries/

# 3. Create the temporary container from the latest build image
docker create --name omn-go-extract omn-go-builder

# 4. Copy the CONTENTS of the bin folder (using the critical /. syntax)
docker cp omn-go-extract:/app/bin/. ./output-binaries/

# 5. Clean up the temporary container
docker rm omn-go-extract

# 6. Show result
ls -l ./output-binaries/
echo "Binaries successfully extracted to perfectly clean ./output-binaries/ directory!"
