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

    # 2. Apply patches – migrate SSH keys from file paths to inline config.json storage

    # 2a. config.go – change GitServerConfig struct field
    old_struct = (
        "type GitServerConfig struct {\n"
        "\tName       string `json:\"name\"`\n"
        "\tURL        string `json:\"url\"`\n"
        "\tSSHKeyPath string `json:\"ssh_key_path\"`\n"
        "\tPassword   string `json:\"password\"`\n"
        "}"
    )
    new_struct = (
        "type GitServerConfig struct {\n"
        "\tName       string `json:\"name\"`\n"
        "\tURL        string `json:\"url\"`\n"
        "\tSSHKeyData string `json:\"ssh_key_data\"`\n"
        "\tPassword   string `json:\"password\"`\n"
        "}"
    )
    patch_file("backend/config.go", old_struct, new_struct)

    # 2b. handlers.go – update the git server card UI to use a textarea for the key
    old_git_card = (
        '\t\tgitHTML += fmt.Sprintf(`\n'
        '\t\t\t<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">\n'
        '\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">\n'
        '\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)\n'
        '\t\t\t\t</label>\n'
        '\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">\n'
        '\t\t\t\t\t<input type="text" id="git_name_%d" name="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_url_%d" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_ssh_%d" name="git_ssh_%d" value="%s" placeholder="SSH Key Path" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="password" id="git_pass_%d" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t</div>\n'
        '\t\t\t</div>`, i, checked, i+1, i, i, gs.Name, i, i, gs.URL, i, i, gs.SSHKeyPath, i, i, gs.Password)'
    )
    new_git_card = (
        '\t\tgitHTML += fmt.Sprintf(`\n'
        '\t\t\t<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">\n'
        '\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">\n'
        '\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)\n'
        '\t\t\t\t</label>\n'
        '\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">\n'
        '\t\t\t\t\t<input type="text" id="git_name_%d" name="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_url_%d" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<textarea id="git_key_%d" name="git_key_%d" placeholder="SSH Private Key" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px; font-family: monospace; min-height: 60px;">%s</textarea>\n'
        '\t\t\t\t\t<input type="password" id="git_pass_%d" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t</div>\n'
        '\t\t\t</div>`, i, checked, i+1, i, i, gs.Name, i, i, gs.URL, i, i, gs.SSHKeyData, i, i, gs.Password)'
    )
    patch_file("backend/handlers.go", old_git_card, new_git_card)

    # 2c. handlers.go – update the handleConfig POST loop to read key data correctly
    old_loop = (
        '\t\t// Update all 5 git server slots\n'
        '\t\tfor i := 0; i < 5; i++ {\n'
        '\t\t\tname := r.FormValue(fmt.Sprintf("git_name_%d", i))\n'
        '\t\t\turl := r.FormValue(fmt.Sprintf("git_url_%d", i))\n'
        '\t\t\tssh := r.FormValue(fmt.Sprintf("git_ssh_%d", i))\n'
        '\t\t\tpass := r.FormValue(fmt.Sprintf("git_pass_%d", i))\n'
        '\t\t\t// update fields if any non‑empty value is supplied (allows clearing)\n'
        '\t\t\tif name != "" || url != "" || ssh != "" || pass != "" {\n'
        '\t\t\t\tappConfig.GitServers[i].Name = name\n'
        '\t\t\t\tappConfig.GitServers[i].URL = url\n'
        '\t\t\t\tappConfig.GitServers[i].SSHKeyPath = ssh\n'
        '\t\t\t\tappConfig.GitServers[i].Password = pass\n'
        '\t\t\t}\n'
        '\t\t}'
    )
    new_loop = (
        '\t\t// Update all 5 git server slots\n'
        '\t\tfor i := 0; i < 5; i++ {\n'
        '\t\t\tname := r.FormValue(fmt.Sprintf("git_name_%d", i))\n'
        '\t\t\turl := r.FormValue(fmt.Sprintf("git_url_%d", i))\n'
        '\t\t\tkeyData := r.FormValue(fmt.Sprintf("git_key_%d", i))\n'
        '\t\t\tpass := r.FormValue(fmt.Sprintf("git_pass_%d", i))\n'
        '\t\t\t// update fields if any non‑empty value is supplied (allows clearing)\n'
        '\t\t\tif name != "" || url != "" || keyData != "" || pass != "" {\n'
        '\t\t\t\tappConfig.GitServers[i].Name = name\n'
        '\t\t\t\tappConfig.GitServers[i].URL = url\n'
        '\t\t\t\tappConfig.GitServers[i].SSHKeyData = keyData\n'
        '\t\t\t\tappConfig.GitServers[i].Password = pass\n'
        '\t\t\t}\n'
        '\t\t}'
    )
    patch_file("backend/handlers.go", old_loop, new_loop)

    # 2d. git_helper.go – replace getSSHAuth to use inline key data
    old_getsshauth = (
        'func getSSHAuth() (transport.AuthMethod, error) {\n'
        '\tsshUser := "git"\n'
        '\tif idx := strings.Index(appConfig.GitServers[appConfig.ActiveGitIndex].URL, "@"); idx != -1 {\n'
        '\t\tsshUser = appConfig.GitServers[appConfig.ActiveGitIndex].URL[:idx]\n'
        '\t}\n'
        '\tlog.Printf("[sync] SSH user: %s", sshUser)\n'
        '\n'
        '\tif appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath == "" {\n'
        '\t\tlog.Printf("[sync] Error: No SSH key configured")\n'
        '\t\treturn nil, fmt.Errorf("no SSH key configured")\n'
        '\t}\n'
        '\n'
        '\tkeyPath := appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath\n'
        '\tif !filepath.IsAbs(keyPath) {\n'
        '\t\tkeyPath = filepath.Join(storageDir, keyPath)\n'
        '\t}\n'
        '\tlog.Printf("[sync] Using SSH key: %s", keyPath)\n'
        '\n'
        '\tinfo, err := os.Stat(keyPath)\n'
        '\tif err != nil {\n'
        '\t\treturn nil, fmt.Errorf("failed to read SSH key: %v", err)\n'
        '\t}\n'
        '\tlog.Printf("[sync] Key file size: %d, mode: %s", info.Size(), info.Mode())\n'
        '\n'
        '\tauth, err := GetInsecureSSHAuth(sshUser, keyPath, appConfig.GitServers[appConfig.ActiveGitIndex].Password)\n'
        '\tif err != nil {\n'
        '\t\treturn nil, fmt.Errorf("GetInsecureSSHAuth error: %v", err)\n'
        '\t}\n'
        '\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer")\n'
        '\treturn auth, nil\n'
        '}'
    )
    new_getsshauth = (
        'func getSSHAuth() (transport.AuthMethod, error) {\n'
        '\tsshUser := "git"\n'
        '\tif idx := strings.Index(appConfig.GitServers[appConfig.ActiveGitIndex].URL, "@"); idx != -1 {\n'
        '\t\tsshUser = appConfig.GitServers[appConfig.ActiveGitIndex].URL[:idx]\n'
        '\t}\n'
        '\tlog.Printf("[sync] SSH user: %s", sshUser)\n'
        '\n'
        '\tkeyData := appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyData\n'
        '\tif keyData == "" {\n'
        '\t\tlog.Printf("[sync] Error: No SSH key configured")\n'
        '\t\treturn nil, fmt.Errorf("no SSH key configured")\n'
        '\t}\n'
        '\n'
        '\tvar signer cryptossh.Signer\n'
        '\tvar err error\n'
        '\tpassphrase := appConfig.GitServers[appConfig.ActiveGitIndex].Password\n'
        '\tif passphrase == "" {\n'
        '\t\tsigner, err = cryptossh.ParsePrivateKey([]byte(keyData))\n'
        '\t} else {\n'
        '\t\tsigner, err = cryptossh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(passphrase))\n'
        '\t}\n'
        '\tif err != nil {\n'
        '\t\treturn nil, fmt.Errorf("failed to parse SSH key: %v", err)\n'
        '\t}\n'
        '\n'
        '\tpublicKeys := &gitssh.PublicKeys{User: sshUser, Signer: signer}\n'
        '\tpublicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{\n'
        '\t\tHostKeyCallback: cryptossh.InsecureIgnoreHostKey(),\n'
        '\t}\n'
        '\tlog.Printf("[sync] SSH auth method created using inline key data")\n'
        '\treturn publicKeys, nil\n'
        '}'
    )
    patch_file("backend/git_helper.go", old_getsshauth, new_getsshauth)

    # 2e. git_helper.go – simplify ensureGitignore (remove SSH key path handling)
    old_gitignore = (
        'func ensureGitignore() {\n'
        '\tgitignorePath := filepath.Join(storageDir, ".gitignore")\n'
        '\tgitignoreBase := "# OMN-Go sync ignore\\nconfig.json\\n*.html\\n"\n'
        '\tif _, err := os.Stat(gitignorePath); os.IsNotExist(err) {\n'
        '\t\tos.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)\n'
        '\t\tlog.Printf("[sync] Created .gitignore")\n'
        '\t}\n'
        '\tif appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath != "" {\n'
        '\t\tkeyPath := appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath\n'
        '\t\tif !filepath.IsAbs(keyPath) {\n'
        '\t\t\tkeyPath = filepath.Join(storageDir, keyPath)\n'
        '\t\t}\n'
        '\t\trelKey, err := filepath.Rel(storageDir, keyPath)\n'
        '\t\tif err == nil && !strings.HasPrefix(relKey, "..") {\n'
        '\t\t\tcurrent, _ := os.ReadFile(gitignorePath)\n'
        '\t\t\tif !strings.Contains(string(current), relKey) {\n'
        '\t\t\t\tnewContent := string(current) + "\\n" + relKey + "\\n"\n'
        '\t\t\t\tos.WriteFile(gitignorePath, []byte(newContent), 0644)\n'
        '\t\t\t\tlog.Printf("[sync] Added %s to .gitignore", relKey)\n'
        '\t\t\t}\n'
        '\t\t}\n'
        '\t}\n'
        '}'
    )
    new_gitignore = (
        'func ensureGitignore() {\n'
        '\tgitignorePath := filepath.Join(storageDir, ".gitignore")\n'
        '\tgitignoreBase := "# OMN-Go sync ignore\\nconfig.json\\n*.html\\n"\n'
        '\tif _, err := os.Stat(gitignorePath); os.IsNotExist(err) {\n'
        '\t\tos.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)\n'
        '\t\tlog.Printf("[sync] Created .gitignore")\n'
        '\t}\n'
        '}'
    )
    patch_file("backend/git_helper.go", old_gitignore, new_gitignore)

    # 3. Print the standardised Git commit message
    commit_msg = (
        "feat(config): store SSH keys inline in config.json instead of file paths\n\n"
        "- GitServerConfig.SSHKeyPath → SSHKeyData (text of private key)\n"
        "- UI card: textarea for pasting key content instead of path input\n"
        "- POST handler saves key data directly; getSSHAuth parses key from memory\n"
        "- Removed file‑path handling from ensureGitignore (keys no longer on disk)\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()