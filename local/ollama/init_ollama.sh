#!/usr/bin/env bash
docker run -d   --name ollama   -v ~/.ollama:/root/.ollama   -v "$(pwd)":/workspace   -p 11434:11434   ollama/ollama
