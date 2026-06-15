#!/bin/bash

# 1. Clean up the old directory (Fixed typo: added 'es' to match the output folder)
rm -rf ./output-binaries/

# 2. Recreate a fresh, empty directory
mkdir -p ./output-binaries/

# 3. Create the temporary container from the latest build image
docker create --name goomn-extract goomn-builder

# 4. Copy the CONTENTS of the bin folder (using the critical /. syntax)
docker cp goomn-extract:/app/bin/. ./output-binaries/

# 5. Clean up the temporary container
docker rm goomn-extract

echo "Binaries successfully extracted to perfectly clean ./output-binaries/ directory!"