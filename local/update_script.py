#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def safe_patch_file(path, old, new):
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
    elif new not in content:
        raise ValueError(f"❌ Patch target not found in {path} (and replacement also missing):\n{old[:120]}")

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add missing import for encoding/json in git_helper.go
    git_helper_path = "backend/git_helper.go"
    git_content = read_file(git_helper_path)
    # The import block currently starts with:
    # import (
    #     "fmt"
    #     "io"
    #     ...
    # We'll insert "encoding/json" after "fmt" if not already present.
    if '"encoding/json"' not in git_content:
        old_imports = '''import (
\t"fmt"
\t"io"'''
        new_imports = '''import (
\t"encoding/json"
\t"fmt"
\t"io"'''
        if old_imports in git_content:
            git_content = git_content.replace(old_imports, new_imports, 1)
            write_file(git_helper_path, git_content)
            print("Added encoding/json import to git_helper.go")
        else:
            # maybe already partially patched differently; try to insert after "fmt"
            # This is a fallback in case the file was already modified.
            # We'll search for line with "fmt" and insert before it.
            lines = git_content.splitlines()
            for i, line in enumerate(lines):
                if line.strip() == '"fmt"':
                    lines.insert(i, '\t"encoding/json"')
                    write_file(git_helper_path, "\n".join(lines))
                    print("Inserted encoding/json import after fmt")
                    break
            else:
                raise ValueError("Could not find 'fmt' import line to insert encoding/json")
    else:
        print("encoding/json already imported in git_helper.go")

    # 3. (The rest of previous patches are already applied, so we just add this missing import)
    commit_msg = (
        "fix(sync): add missing encoding/json import for handleSyncPreview\n\n"
        "- Resolved build error in git_helper.go\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()