#!/bin/bash

# Define color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

OUTPUT_FILE="doc/GoOMN_1.1.0_Context.md"

echo -e "${BLUE}=======================================${NC}"
echo -e "${YELLOW}  GoOMN AI Context Generator${NC}"
echo -e "${BLUE}=======================================${NC}"

# 1. Write the precise AI prompt at the top of the file
cat << 'PROMPT_EOF' > "$OUTPUT_FILE"
Here is the current state of the GoOMN project. We are currently at Version 1.1.0 (Android version code 10100).

Recently, we migrated the Android app to a Java WebView wrapper using a Dockerized Gradle build (eliminating the old 5MB APK limit while strictly keeping NO AppCompat). We also added offline support for KaTeX and Highlight.js, implemented a dynamic JS Console Interceptor UI with a Clear button, and fixed directory-based Content-Type routing.

Below is the complete current codebase and the master `initial_prompt.md`. Please review them and acknowledge that you are ready for my next request. Remember to strictly follow the Turn 2 Python patching output format.

PROMPT_EOF

# 2. Define the mandatory files (including the Android build.gradle)
FILES=(
    "doc/initial_prompt.md"
    "Dockerfile"
    "go.mod"
    "main_desktop.go"
    "backend/server.go"
    "backend/frontend/index.html"
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
    echo -e "\n### $ACTUAL_PATH END" >> "$OUTPUT_FILE"
    
    echo -e "${GREEN}[SUCCESS]${NC}"
done

echo -e "${BLUE}=======================================${NC}"
echo -e "${GREEN}Done!${NC} The file ${YELLOW}$OUTPUT_FILE${NC} has been generated."
echo -e "You can now drag and drop just this ONE text file into the new chat!"
