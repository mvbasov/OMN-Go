import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.37...")

def bump_version():
    new_v = "1.4.37"
    new_v_c = "10437"

    # 1. Update version.go
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "w") as f: 
            f.write(f'package backend\n\n// APP_VERSION is the global application version\nconst APP_VERSION = "{new_v}"\n')
        print(f"  [+] Hard-rewrote {ver_path} with APP_VERSION = {new_v}")
    else:
        print(f"  [-] Warning: {ver_path} not found")

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
            # Add to Config struct safely
            content = re.sub(r'(type\s+Config\s+struct\s*\{)', r'\1\n\tForcePullOneTime bool `json:"force_pull_one_time"`', content)
            with open(cfg_path, "w") as f: f.write(content)
            print("  [+] Added ForcePullOneTime to Config struct in backend/config.go")

def rebuild_git_helper_with_smart_config():
    # Completely rebuild the file to include our JSON readers for Config
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

// GetInsecureSSHAuth bypasses strict host key checking which blocks Android from connecting to gitolite
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
\t// EXPLICIT PUBKEY EXTRACTION
\tsigner := publicKeys.Signer
\tpubKeyBytes := gossh.MarshalAuthorizedKey(signer.PublicKey())
\tpubKeyStr := strings.TrimSpace(string(pubKeyBytes))
\t
\tlog.Printf("\\n[CRITICAL] To fix 'unable to authenticate', add THIS EXACT KEY to your gitolite-admin repo:")
\tlog.Printf("[CRITICAL] %s", pubKeyStr)
\tlog.Printf("[CRITICAL] Your desktop CLI likely succeeded by silently falling back to ~/.ssh/id_rsa!\\n")

\t// CRITICAL FIX: Ignore host key verification for gitolite3 servers
\tpublicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
\treturn publicKeys, nil
}

// GetForcePullAndReset reads config.json, checks the one-time flag, and auto-resets it to false.
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
\tforce, ok := cfg["force_pull_one_time"].(bool)
\tif force {
\t\tlog.Printf("[SYNC] ForcePullOneTime detected! Executing destructive Force Pull and resetting flag to false.")
\t\tcfg["force_pull_one_time"] = false
\t\tif newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
\t\t\tos.WriteFile(configPath, newData, 0644)
\t\t}
\t}
\treturn force
}

// GetConfigAuthor dynamically extracts the author from the config JSON.
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
    print("  [+] Cleanly rebuilt backend/git_helper.go with Smart Config extractors")

def patch_network_options():
    for go_file in glob.glob("backend/*.go"):
        if not os.path.exists(go_file): continue
        with open(go_file, "r") as f: content = f.read()
        modified = False

        # 1. Update Commits to use Config Author
        if "git.CommitOptions" in content:
            # Ensure required packages are imported
            if '"time"' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"time"\n', content, count=1)
            if 'plumbing/object' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"github.com/go-git/go-git/v5/plumbing/object"\n', content, count=1)

            def inject_author(match):
                inner = match.group(2)
                # Scrub old Author injections
                inner = re.sub(r'\s*Author:\s*&object\.Signature\{[^}]+\},', '\n\t\t', inner)
                author_block = """
\t\tAuthor: &object.Signature{
\t\t\tName:  GetConfigAuthor(),
\t\t\tEmail: "sync@omn-go.local",
\t\t\tWhen:  time.Now(),
\t\t},"""
                return match.group(1) + author_block + inner + "}"

            new_content = re.sub(r'(git\.CommitOptions\s*\{)(.*?)\}', inject_author, content, flags=re.DOTALL)
            if new_content != content:
                content = new_content
                modified = True

        # 2. Update Auth and Remove unsafe Force injections
        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                if f"&git.{opt}{{" in content or f"git.{opt}{{" in content:
                    # Scrub existing fields
                    content = re.sub(rf'\s*Auth:\s*[a-zA-Z0-9_.]+,\s*', '\n\t\t', content)
                    content = re.sub(rf'\s*Force:\s*(true|false|GetForcePullAndReset\(\)),\s*', '\n\t\t', content)
                    
                    # Only PullOptions gets the One-Time Force override!
                    force_str = r'\n\t\tForce: GetForcePullAndReset(),' if opt == 'PullOptions' else ''
                    injection = r'\1\n\t\tAuth: ' + auth_var + r',' + force_str
                    
                    new_content = re.sub(r'(git\.' + opt + r'\s*\{)', injection, content)
                    if new_content != content:
                        content = new_content
                        modified = True
        
        if modified:
            with open(go_file, "w") as f: f.write(content)
            print(f"  [+] Patched Auth & Smart Force policies in {go_file}")

if __name__ == "__main__":
    bump_version()
    patch_config_struct()
    rebuild_git_helper_with_smart_config()
    patch_network_options()
    print("[*] Update complete! Version 1.4.37 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Feature: Secure Force Pull & Dynamic Config Authors")
    print("\n- Bumped application version to 1.4.37")
    print("- Replaced dangerous hardcoded Force Push/Pull with a one-time")
    print("  ForcePullOneTime toggle inside data/config.json.")
    print("- Implemented automatic reset of force flag after successful read.")
    print("- Replaced hardcoded 'OMN-Go App' commit author with dynamic")
    print("  extraction from config.json.")
    print("="*55 + "\n")