#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*. Raise ValueError if *old* missing."""
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

    # 2. Add gitignore import to git_helper.go
    old_import = (
        '\t"sort"\n'
        '\t"strings"\n'
        '\t"time"'
    )
    new_import = (
        '\t"sort"\n'
        '\t"strings"\n'
        '\t"github.com/go-git/go-git/v5/plumbing/format/gitignore"\n'
        '\t"time"'
    )
    patch_file("backend/git_helper.go", old_import, new_import)

    # 3. Replace writeTreeFromDir with gitignore‑aware version
    old_tree = (
        'func writeTreeFromDir(dir string, storer storage.Storer) (plumbing.Hash, error) {\n'
        '\tfiles, err := os.ReadDir(dir)\n'
        '\tif err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\t// Sort directory entries for deterministic order\n'
        '\tsort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })\n'
        '\tentries := []object.TreeEntry{}\n'
        '\tfor _, f := range files {\n'
        '\t\tif f.Name() == ".git" {\n'
        '\t\t\tcontinue\n'
        '\t\t}\n'
        '\t\tfullPath := filepath.Join(dir, f.Name())\n'
        '\t\tif f.IsDir() {\n'
        '\t\t\tsubTreeHash, err := writeTreeFromDir(fullPath, storer)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0040000,\n'
        '\t\t\t\tHash: subTreeHash,\n'
        '\t\t\t})\n'
        '\t\t} else {\n'
        '\t\t\tdata, err := os.ReadFile(fullPath)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tblobObj := storer.NewEncodedObject()\n'
        '\t\t\tblobObj.SetType(plumbing.BlobObject)\n'
        '\t\t\tblobObj.SetSize(int64(len(data)))\n'
        '\t\t\tw, err := blobObj.Writer()\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tif _, err = w.Write(data); err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tw.Close()\n'
        '\t\t\tblobHash, err := storer.SetEncodedObject(blobObj)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0100644,\n'
        '\t\t\t\tHash: blobHash,\n'
        '\t\t\t})\n'
        '\t\t}\n'
        '\t}\n'
        '\t// Build tree object\n'
        '\ttreeObj := object.Tree{Entries: entries}\n'
        '\tencoded := storer.NewEncodedObject()\n'
        '\tif err := treeObj.Encode(encoded); err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\treturn storer.SetEncodedObject(encoded)\n'
        '}'
    )

    new_tree = (
        'func writeTreeFromDir(dir string, storer storage.Storer) (plumbing.Hash, error) {\n'
        '\t// Load .gitignore patterns\n'
        '\tvar ps []gitignore.Pattern\n'
        '\tgitignorePath := filepath.Join(storageDir, ".gitignore")\n'
        '\tif data, err := os.ReadFile(gitignorePath); err == nil {\n'
        '\t\tlines := strings.Split(string(data), "\\n")\n'
        '\t\tfor _, line := range lines {\n'
        '\t\t\tline = strings.TrimSpace(line)\n'
        '\t\t\tif line == "" || strings.HasPrefix(line, "#") {\n'
        '\t\t\t\tcontinue\n'
        '\t\t\t}\n'
        '\t\t\tps = append(ps, gitignore.ParsePattern(line, nil))\n'
        '\t\t}\n'
        '\t}\n'
        '\tmatcher := gitignore.NewMatcher(ps)\n'
        '\n'
        '\tfiles, err := os.ReadDir(dir)\n'
        '\tif err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\t// Sort directory entries for deterministic order\n'
        '\tsort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })\n'
        '\tentries := []object.TreeEntry{}\n'
        '\tfor _, f := range files {\n'
        '\t\tif f.Name() == ".git" {\n'
        '\t\t\tcontinue\n'
        '\t\t}\n'
        '\t\tfullPath := filepath.Join(dir, f.Name())\n'
        '\t\t// Compute relative path from storageDir\n'
        '\t\trelPath, err := filepath.Rel(storageDir, fullPath)\n'
        '\t\tif err != nil {\n'
        '\t\t\tcontinue\n'
        '\t\t}\n'
        '\t\tif matcher.Match(strings.Split(relPath, string(filepath.Separator)), f.IsDir()) {\n'
        '\t\t\tcontinue\n'
        '\t\t}\n'
        '\t\tif f.IsDir() {\n'
        '\t\t\tsubTreeHash, err := writeTreeFromDir(fullPath, storer)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0040000,\n'
        '\t\t\t\tHash: subTreeHash,\n'
        '\t\t\t})\n'
        '\t\t} else {\n'
        '\t\t\tdata, err := os.ReadFile(fullPath)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tblobObj := storer.NewEncodedObject()\n'
        '\t\t\tblobObj.SetType(plumbing.BlobObject)\n'
        '\t\t\tblobObj.SetSize(int64(len(data)))\n'
        '\t\t\tw, err := blobObj.Writer()\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tif _, err = w.Write(data); err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tw.Close()\n'
        '\t\t\tblobHash, err := storer.SetEncodedObject(blobObj)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0100644,\n'
        '\t\t\t\tHash: blobHash,\n'
        '\t\t\t})\n'
        '\t\t}\n'
        '\t}\n'
        '\t// Build tree object\n'
        '\ttreeObj := object.Tree{Entries: entries}\n'
        '\tencoded := storer.NewEncodedObject()\n'
        '\tif err := treeObj.Encode(encoded); err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\treturn storer.SetEncodedObject(encoded)\n'
        '}'
    )

    patch_file("backend/git_helper.go", old_tree, new_tree)

    # 4. Print commit message
    commit_msg = (
        "fix(sync): respect .gitignore during tree building to prevent deletion of ignored files\n\n"
        "- writeTreeFromDir now parses .gitignore patterns and excludes matching files/dirs\n"
        "- Ignored files are never committed; force download resets only tracked content\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()