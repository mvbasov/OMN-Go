#!/usr/bin/env python3
import re, os, sys

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Apply the fix for force checkout (add Keep:true to preserve untracked files)
    old_force_checkout = (
        "\t\terr = wTree.Checkout(&git.CheckoutOptions{\n"
        "\t\t\tHash:  ref.Hash(),\n"
        "\t\t\tForce: true,\n"
        "\t\t})"
    )
    new_force_checkout = (
        "\t\terr = wTree.Checkout(&git.CheckoutOptions{\n"
        "\t\t\tHash:  ref.Hash(),\n"
        "\t\t\tForce: true,\n"
        "\t\t\tKeep:  true,\n"
        "\t\t})"
    )

    # 2. Replace the non‑force pull path with fetch + checkout with Keep:true
    old_pull_path = (
        '\t} else {\n'
        '\t\tlog.Printf("[sync] Pulling from origin master")\n'
        '\t\terr := wTree.Pull(&git.PullOptions{\n'
        '\t\t\tRemoteName:    "origin",\n'
        '\t\t\tAuth:          auth,\n'
        '\t\t\tReferenceName: plumbing.NewBranchReferenceName("master"),\n'
        '\t\t\tSingleBranch:  true,\n'
        '\t\t})\n'
        '\t\tif err != nil && err != git.NoErrAlreadyUpToDate && !strings.Contains(err.Error(), "couldn\'t find remote ref") {\n'
        '\t\t\treturn fmt.Errorf("pull failed: %v", err)\n'
        '\t\t}\n'
        '\t}'
    )
    new_pull_path = (
        '\t} else {\n'
        '\t\tlog.Printf("[sync] Fetching from origin master for merge")\n'
        '\t\terr := repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth})\n'
        '\t\tif err != nil && err != git.NoErrAlreadyUpToDate {\n'
        '\t\t\treturn fmt.Errorf("fetch failed: %v", err)\n'
        '\t\t}\n'
        '\n'
        '\t\tref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", "master"), true)\n'
        '\t\tif err != nil {\n'
        '\t\t\treturn fmt.Errorf("failed to find origin/master: %v", err)\n'
        '\t\t}\n'
        '\n'
        '\t\terr = wTree.Checkout(&git.CheckoutOptions{\n'
        '\t\t\tHash:  ref.Hash(),\n'
        '\t\t\tForce: false,\n'
        '\t\t\tKeep:  true,\n'
        '\t\t})\n'
        '\t\tif err != nil {\n'
        '\t\t\treturn fmt.Errorf("checkout failed: %v", err)\n'
        '\t\t}\n'
        '\n'
        '\t\trepo.Storer.SetReference(plumbing.NewHashReference(\n'
        '\t\t\tplumbing.ReferenceName("refs/heads/master"), ref.Hash()))\n'
        '\t\trepo.Storer.SetReference(plumbing.NewSymbolicReference(\n'
        '\t\t\tplumbing.HEAD, plumbing.ReferenceName("refs/heads/master")))\n'
        '\t}'
    )

    try:
        patch_file("backend/git_helper.go", old_force_checkout, new_force_checkout)
        patch_file("backend/git_helper.go", old_pull_path, new_pull_path)
    except ValueError as e:
        print("One or more patches already applied – nothing to do.")
        sys.exit(0)

    # 3. Auto‑detect current version and bump it
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

    # 4. Output commit message
    commit_msg = (
        "fix(sync): preserve all untracked files during pull and force download\n\n"
        "- Replaced git.Pull (which discards untracked files) with fetch + checkout Keep:true\n"
        "- Added Keep:true to the force reset checkout to stop deleting dynamic local files\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()