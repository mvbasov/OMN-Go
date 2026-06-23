#!/usr/bin/env bash
docker exec -it ollama /bin/sh -c cd /workspace && find . -type f \( -name '*.go' -o -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.md' \) -exec cat {} \; | ollama run carstenuhlig/omnicoder-9b:q4_k_m 'Analyze this codebase structure and create User Manual in Markdown format'
