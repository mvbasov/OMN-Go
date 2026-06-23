#!/usr/bin/env python3
"""OMN-Go 1.3.28 → 1.3.29: fix Go compilation errors (unknown escape sequences)."""

import os

def patch_file(path, old, new):
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    if old not in text:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}...")
    text = text.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)

def update_application():
    # ========== VERSION BUMPS ==========
    patch_file("backend/version.go",
               'APP_VERSION = "1.3.28"',
               'APP_VERSION = "1.3.29"')
    patch_file("android/app/build.gradle",
               "versionCode 10328",
               "versionCode 10329")
    patch_file("android/app/build.gradle",
               'versionName "1.3.28"',
               'versionName "1.3.29"')

    # ========== FIX 1: server.go - replace invalid \' escape in script injection ==========
    old_server_inject = '''scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode===\\'function\\') toggleMode(); }, 120);</script>"'''
    new_server_inject = '''scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>"'''
    patch_file("backend/server.go", old_server_inject, new_server_inject)

    # ========== FIX 2: handlers.go - same invalid escape ==========
    old_handler_inject = '''scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode===\\'function\\') toggleMode(); }, 120);</script>"'''
    new_handler_inject = '''scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>"'''
    patch_file("backend/handlers.go", old_handler_inject, new_handler_inject)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "fix(build): remove invalid escape sequences from script injection strings\n\n"
        "The previous patch used backslash-single-quote (\\') inside double-quoted\n"
        "Go strings, which is not a recognized escape sequence.  Replaced those\n"
        "with plain single quotes, as they are safe inside double-quoted strings.\n"
        "Fixes compilation errors in handlers.go and server.go.\n\n"
        "Version bumped to 1.3.29"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()