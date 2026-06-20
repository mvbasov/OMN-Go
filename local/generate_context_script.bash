#!/bin/bash

# Define color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

OUTPUT_FILE="doc/OMN-Go_1.3.16_Context.md"

echo -e "${BLUE}=======================================${NC}"
echo -e "${YELLOW}  OMN-Go AI Context Generator${NC}"
echo -e "${BLUE}=======================================${NC}"

# 1. Write the precise AI prompt at the top of the file
cat << 'PROMPT_EOF' > "$OUTPUT_FILE"
Here is the current state of the OMN-Go project. We are currently at Version 1.3.16 (Android version code 10316).

Below is the complete current codebase and the master `initial_prompt.md`. Please review them and acknowledge that you are ready for my next request. Remember to strictly follow the Turn 2 Python patching output format. Application version need to be updated on every changes.

We are redy to implement discussed blueprint
PROMPT_EOF

# 2. Define the mandatory files (including the Android build.gradle)
FILES=(
    "doc/initial_prompt.md"
    "Dockerfile"
    "go.mod"
    "main_desktop.go"
    "backend/server.go"
    "backend/frontend/index.html"
    "backend/frontend/html/css/omn-go-core.css"
    "backend/frontend/html/js/omn-go-core.js"
    "android/build.gradle"
    "android/settings.gradle"
    "android/app/build.gradle"
    "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    "android/app/src/main/AndroidManifest.xml"
)

# 3. Append each file cleanly with Markdown code blocks
for f in "${FILES[@]}"; do
    # Handle path variations gracefully (in case run from inside backend/)
    ACTUAL_PATH="$f"
    if [ ! -f "$ACTUAL_PATH" ]; then
        BASENAME=$(basename "$f")
        if [ -f "$BASENAME" ]; then
            ACTUAL_PATH="$BASENAME"
        elif [ -f "../$f" ]; then
            ACTUAL_PATH="../$f"
        else
            echo -e "${RED}[WARNING]${NC} Could not find $f. Skipping..."
            continue
        fi
    fi

    echo -ne "Appending ${ACTUAL_PATH}... "
    
    echo -e "\n### $ACTUAL_PATH START" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    cat "$ACTUAL_PATH" >> "$OUTPUT_FILE"
    echo -e "\n\`\`\`" >> "$OUTPUT_FILE"
    echo -e "\n### $ACTUAL_PATH END\n" >> "$OUTPUT_FILE"

    echo -e "${GREEN}Done${NC}"
done
echo -e "\n### FULL PR0JECT DIRECTORY TREE START\n" >> "$OUTPUT_FILE"
tree >> "$OUTPUT_FILE"
echo -e "\n### FULL PR0JECT DIRECTORY TREE END\n" >> "$OUTPUT_FILE"

echo -e "${BLUE}=======================================${NC}"
echo -e "${GREEN}[SUCCESS]${NC} Context file generated at: ${YELLOW}$OUTPUT_FILE${NC}"
echo -e "Drag this file into the new chat window to continue building!"
