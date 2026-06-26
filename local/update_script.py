import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.38...")

def bump_version():
    new_v = "1.4.38"
    new_v_c = "10438"

    # 1. Update version.go
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "w") as f: 
            f.write(f'package backend\n\n// APP_VERSION is the global application version\nconst APP_VERSION = "{new_v}"\n')
        print(f"  [+] Hard-rewrote {ver_path} with APP_VERSION = {new_v}")

    # 2. Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f: c = f.read()
        c = re.sub(r'versionCode\s+\d+', f'versionCode {new_v_c}', c)
        c = re.sub(r'versionName\s+"[^"]+"', f'versionName "{new_v}"', c)
        with open(gradle_path, "w") as f: f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def patch_config_struct():
    cfg_path = "backend/config.go"
    if os.path.exists(cfg_path):
        with open(cfg_path, "r") as f: content = f.read()
        if "ForcePullOneTime" not in content and "type Config struct" in content:
            content = re.sub(r'(type\s+Config\s+struct\s*\{)', r'\1\n\tForcePullOneTime bool `json:"force_pull_one_time"`', content)
            with open(cfg_path, "w") as f: f.write(content)
            print("  [+] Added ForcePullOneTime to Config struct")

def rebuild_git_helper_with_smart_config():
    # Completely rebuild the file to fix the `ok` variable and handle JSON string fallbacks
    helper_path = "backend/git_helper.go"
    helper_code = """package backend

import (
\t"encoding/json"
\t"log"
\t"os"
\t"strings"
\t"github.com/go-git/go-git/v5/plumbing/transport/ssh"
\tgossh "golang.org/x/crypto/ssh"
)

func GetInsecureSSHAuth(sshUser, privateKeyPath, password string) (*ssh.PublicKeys, error) {
\t_, err := os.Stat(privateKeyPath)
\tif err != nil {
\t\treturn nil, err
\t}
\tpublicKeys, err := ssh.NewPublicKeysFromFile(sshUser, privateKeyPath, password)
\tif err != nil {
\t\treturn nil, err
\t}
\t
\tsigner := publicKeys.Signer
\tpubKeyBytes := gossh.MarshalAuthorizedKey(signer.PublicKey())
\tpubKeyStr := strings.TrimSpace(string(pubKeyBytes))
\t
\tlog.Printf("\\n[CRITICAL] To fix 'unable to authenticate', add THIS EXACT KEY to your gitolite-admin repo:")
\tlog.Printf("[CRITICAL] %s", pubKeyStr)
\tlog.Printf("[CRITICAL] Your desktop CLI likely succeeded by silently falling back to ~/.ssh/id_rsa!\\n")

\tpublicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
\treturn publicKeys, nil
}

func GetForcePullAndReset() bool {
\tconfigPath := "data/config.json"
\tconfigData, err := os.ReadFile(configPath)
\tif err != nil {
\t\treturn false
\t}
\tvar cfg map[string]interface{}
\tif err := json.Unmarshal(configData, &cfg); err != nil {
\t\treturn false
\t}
\t
\tforce := false
\t// FIX: Safely parse both actual booleans and stringified booleans from the JS UI
\tif valBool, ok := cfg["force_pull_one_time"].(bool); ok {
\t\tforce = valBool
\t} else if valStr, ok := cfg["force_pull_one_time"].(string); ok {
\t\tforce = (valStr == "true" || valStr == "on")
\t}
\t
\tif force {
\t\tlog.Printf("[SYNC] ForcePullOneTime detected! Executing destructive Force Pull and resetting flag to false.")
\t\tcfg["force_pull_one_time"] = false
\t\tif newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
\t\t\tos.WriteFile(configPath, newData, 0644)
\t\t}
\t}
\treturn force
}

func GetConfigAuthor() string {
\tauthor := "OMN-Go App"
\tconfigPath := "data/config.json"
\tif configData, err := os.ReadFile(configPath); err == nil {
\t\tvar cfg map[string]interface{}
\t\tif json.Unmarshal(configData, &cfg) == nil {
\t\t\tif val, ok := cfg["author"].(string); ok && val != "" {
\t\t\t\tauthor = val
\t\t\t} else if val, ok := cfg["Author"].(string); ok && val != "" {
\t\t\t\tauthor = val
\t\t\t}
\t\t}
\t}
\treturn author
}
"""
    with open(helper_path, "w") as f: 
        f.write(helper_code)
    print("  [+] Rebuilt backend/git_helper.go (Fixed 'ok declared not used')")

def patch_network_options():
    for go_file in glob.glob("backend/*.go"):
        if not os.path.exists(go_file): continue
        with open(go_file, "r") as f: content = f.read()
        modified = False

        if "git.CommitOptions" in content:
            if '"time"' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"time"\n', content, count=1)
            if 'plumbing/object' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"github.com/go-git/go-git/v5/plumbing/object"\n', content, count=1)

            def inject_author(match):
                inner = match.group(2)
                # FIX: Bulletproof line-by-line scrubber to prevent 'duplicate Author'
                lines = inner.split('\n')
                clean_lines = []
                in_author_block = False
                for line in lines:
                    if 'Author:' in line:
                        if '{' in line and '}' not in line:
                            in_author_block = True
                        continue
                    if in_author_block:
                        if '}' in line:
                            in_author_block = False
                        continue
                    clean_lines.append(line)
                
                clean_inner = '\n'.join(clean_lines)
                author_block = """
\t\tAuthor: &object.Signature{
\t\t\tName:  GetConfigAuthor(),
\t\t\tEmail: "sync@omn-go.local",
\t\t\tWhen:  time.Now(),
\t\t},"""
                return match.group(1) + author_block + clean_inner + "}"

            new_content = re.sub(r'(git\.CommitOptions\s*\{)(.*?)\}', inject_author, content, flags=re.DOTALL)
            if new_content != content:
                content = new_content
                modified = True

        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                if f"&git.{opt}{{" in content or f"git.{opt}{{" in content:
                    content = re.sub(rf'\s*Auth:\s*[a-zA-Z0-9_.]+,\s*', '\n\t\t', content)
                    content = re.sub(rf'\s*Force:\s*(true|false|GetForcePullAndReset\(\)),\s*', '\n\t\t', content)
                    
                    force_str = r'\n\t\tForce: GetForcePullAndReset(),' if opt == 'PullOptions' else ''
                    injection = r'\1\n\t\tAuth: ' + auth_var + r',' + force_str
                    
                    new_content = re.sub(r'(git\.' + opt + r'\s*\{)', injection, content)
                    if new_content != content:
                        content = new_content
                        modified = True
        
        if modified:
            with open(go_file, "w") as f: f.write(content)
            print(f"  [+] Patched Network Auth & Scrubbed Duplicate Authors in {go_file}")

def patch_config_ui():
    # Dynamically inject the checkbox into the Config HTML wherever the Author field exists
    for file_path in glob.glob("backend/*.go") + glob.glob("backend/frontend/*.html"):
        if not os.path.exists(file_path): continue
        with open(file_path, "r") as f: content = f.read()
        
        if "force_pull_one_time" not in content and re.search(r'name=["\']?[aA]uthor["\']?', content):
            pattern = r'(<input[^>]+name=["\']?[aA]uthor["\']?[^>]*>)'
            checkbox = r'\1\n\t\t\t<div style="margin-top: 15px;">\n\t\t\t\t<label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">\n\t\t\t\t\t<input type="checkbox" name="force_pull_one_time" value="true">\n\t\t\t\t\t<span>⚠ Force Initial Pull (One Time Overwrite)</span>\n\t\t\t\t</label>\n\t\t\t</div>'
            content = re.sub(pattern, checkbox, content)
            with open(file_path, "w") as f: f.write(content)
            print(f"  [+] Injected Force Pull Checkbox UI into {file_path}")

if __name__ == "__main__":
    bump_version()
    patch_config_struct()
    rebuild_git_helper_with_smart_config()
    patch_network_options()
    patch_config_ui()
    print("[*] Update complete! Version 1.4.38 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Resolve Go compilation strictness & Add Config UI")
    print("\n- Bumped application version to 1.4.38")
    print("- Fixed 'declared and not used: ok' in GetForcePullAndReset")
    print("- Implemented multi-line scrub to resolve 'duplicate field Author'")
    print("- Injected dynamic Force Pull Checkbox directly into the Config UI")
    print("="*55 + "\n")