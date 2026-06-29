#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*.  Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def patch_file_if_needed(path, old, new):
    """Apply patch if *old* exists, otherwise skip.  Returns True if applied."""
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
        return True
    return False

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version and bump
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    new_code = int(new_ver.replace(".", ""))

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    gradle = gradle.replace(f"versionCode {old_code}", f"versionCode {new_code}")
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix the call site that still uses single return value
    old_call = (
        "\tif err := commitLocalChanges(repo, wTree); err != nil {\n"
        "\t\thttp.Error(w, fmt.Sprintf(\"Commit failed: %v\", err), 500)\n"
        "\t\treturn\n"
        "\t}\n"
        "\n"
        "\tif action == \"download\" {"
    )
    new_call = (
        "\tcommitted, err := commitLocalChanges(repo, wTree)\n"
        "\tif err != nil {\n"
        "\t\thttp.Error(w, fmt.Sprintf(\"Commit failed: %v\", err), 500)\n"
        "\t\treturn\n"
        "\t}\n"
        "\tif !committed && action == \"upload\" {\n"
        "\t\tw.Write([]byte(\"Nothing to push\"))\n"
        "\t\treturn\n"
        "\t}\n"
        "\n"
        "\tif action == \"download\" {"
    )

    applied1 = patch_file_if_needed("backend/handlers.go", old_call, new_call)
    applied2 = patch_file_if_needed("backend/git_helper.go", old_call, new_call)

    if applied1 or applied2:
        print("✓ Fixed call site(s) for commitLocalChanges")
    else:
        # Might already be fixed; just ensure no compile error
        pass

    # 3. Fix the safeCommit recursion bug (calling itself without termination)
    old_safe_commit = (
        "func safeCommit(w *git.Worktree, msg string, opts *git.CommitOptions) (plumbing.Hash, error) {\n"
        "\tstatus, err := w.Status()\n"
        "\tif err != nil {\n"
        "\t\treturn plumbing.ZeroHash, err\n"
        "\t}\n"
        "\tif status.IsClean() {\n"
        "\t\t// Strong check: explicitly bypass commit if tree is perfectly clean\n"
        "\t\treturn plumbing.ZeroHash, nil\n"
        "\t}\n"
        "\treturn safeCommit(w, msg, opts)\n"
        "}"
    )
    new_safe_commit = (
        "func safeCommit(w *git.Worktree, msg string, opts *git.CommitOptions) (plumbing.Hash, error) {\n"
        "\tstatus, err := w.Status()\n"
        "\tif err != nil {\n"
        "\t\treturn plumbing.ZeroHash, err\n"
        "\t}\n"
        "\tif status.IsClean() {\n"
        "\t\t// Strong check: explicitly bypass commit if tree is perfectly clean\n"
        "\t\treturn plumbing.ZeroHash, nil\n"
        "\t}\n"
        "\treturn w.Commit(msg, opts)\n"
        "}"
    )
    # Apply only if the old version exists
    if patch_file_if_needed("backend/git_helper.go", old_safe_commit, new_safe_commit):
        print("✓ Fixed safeCommit recursion")

    # 4. Frontend JS – show server response message (if not already done)
    old_js = (
        "            if (res.ok) {\n"
        "                if (confirm(capAction + ' complete!\\n\\nWould you like to reload the page now to see updated content (console will be reset)?')) {\n"
        "                    window.location.reload();\n"
        "                }\n"
        "            } else {"
    )
    new_js = (
        "            if (res.ok) {\n"
        "                let msg = await res.text();\n"
        "                if (msg.includes('Nothing to push')) {\n"
        "                    alert(msg);\n"
        "                } else if (confirm(msg + '\\n\\nWould you like to reload the page now to see updated content (console will be reset)?')) {\n"
        "                    window.location.reload();\n"
        "                }\n"
        "            } else {"
    )
    if patch_file_if_needed("backend/frontend/html/js/omn-go-sse.js", old_js, new_js):
        print("✓ Updated frontend sync dialogue")

    # 5. Print commit message
    commit_msg = (
        "fix(sync): correct commitLocalChanges call and fix safeCommit recursion\n\n"
        "- Update handleSync call to capture (bool, error) returned by commitLocalChanges\n"
        "- Replace infinite recursion in safeCommit with actual w.Commit call\n"
        "- Frontend shows 'Nothing to push' when upload has nothing to do\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()