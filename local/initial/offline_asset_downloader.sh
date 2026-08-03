#!/bin/bash

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=======================================${NC}"
echo -e "${YELLOW}  GoOMN Offline Asset Downloader${NC}"
echo -e "${BLUE}=======================================${NC}"

JS_DIR="backend/frontend/html/js"
CSS_DIR="backend/frontend/html/css"
FONT_DIR="backend/frontend/html/css/fonts"

mkdir -p "$JS_DIR"
mkdir -p "$CSS_DIR"
mkdir -p "$FONT_DIR"

download_file() {
    local url=$1
    local dest=$2
    local name=$3

    echo -ne "Downloading ${name}... "
    if curl -sL --fail "$url" -o "$dest"; then
        echo -e "${GREEN}[SUCCESS]${NC}"
    else
        echo -e "${RED}[FAILED]${NC}"
        echo -e "${RED}Error downloading $name from $url${NC}"
        exit 1
    fi
}

download_file "https://cdnjs.cloudflare.com/ajax/libs/github-markdown-css/5.5.0/github-markdown.min.css" "$CSS_DIR/markdown.css" "Markdown CSS"

download_file "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js" "$JS_DIR/highlight.min.js" "Highlight.js"
download_file "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/default.min.css" "$CSS_DIR/highlight.default.min.css" "Highlight.js CSS"

download_file "https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.js" "$JS_DIR/katex.min.js" "KaTeX.js"
download_file "https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/contrib/auto-render.min.js" "$JS_DIR/auto-render.min.js" "KaTeX Auto-Render"
download_file "https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.css" "$CSS_DIR/katex.min.css" "KaTeX CSS"

KATEX_FONTS=(
    "KaTeX_AMS-Regular.woff2" "KaTeX_Caligraphic-Bold.woff2" "KaTeX_Caligraphic-Regular.woff2"
    "KaTeX_Fraktur-Bold.woff2" "KaTeX_Fraktur-Regular.woff2" "KaTeX_Main-Bold.woff2"
    "KaTeX_Main-BoldItalic.woff2" "KaTeX_Main-Italic.woff2" "KaTeX_Main-Regular.woff2"
    "KaTeX_Math-BoldItalic.woff2" "KaTeX_Math-Italic.woff2" "KaTeX_SansSerif-Bold.woff2"
    "KaTeX_SansSerif-Italic.woff2" "KaTeX_SansSerif-Regular.woff2" "KaTeX_Script-Regular.woff2"
    "KaTeX_Size1-Regular.woff2" "KaTeX_Size2-Regular.woff2" "KaTeX_Size3-Regular.woff2"
    "KaTeX_Size4-Regular.woff2" "KaTeX_Typewriter-Regular.woff2"
)

echo -e "${BLUE}Downloading KaTeX Fonts...${NC}"
for font in "${KATEX_FONTS[@]}"; do
    download_file "https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/$font" "$FONT_DIR/$font" "Font: $font"
done

download_file "https://fonts.gstatic.com/s/materialicons/v143/flUhRq6tzZclQEJ-Vdg-IuiaDsNc.woff2" "$FONT_DIR/material-icons.woff2" "Material Icons Font"

echo -e "${BLUE}=======================================${NC}"
echo -e "${GREEN}All assets successfully downloaded to ${JS_DIR}, ${CSS_DIR}, and ${FONT_DIR}!${NC}"
