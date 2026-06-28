import os
import re
import sys

VERSION = "1.5.6"
VERSION_CODE = "10506"

def read_file(filepath):
    if not os.path.exists(filepath):
        return None
    with open(filepath, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(filepath, content):
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[+] Successfully patched {filepath}")

def bump_versions():
    # 1. Bump version.go
    v_path = os.path.join("backend", "version.go")
    content = read_file(v_path)
    if content:
        content = re.sub(r'APP_VERSION\s*=\s*".*?"', f'APP_VERSION = "{VERSION}"', content)
        write_file(v_path, content)
    
    # 2. Bump build.gradle
    b_path = os.path.join("android", "app", "build.gradle")
    content = read_file(b_path)
    if content:
        content = re.sub(r'versionCode\s+\d+', f'versionCode {VERSION_CODE}', content)
        content = re.sub(r'versionName\s+".*?"', f'versionName "{VERSION}"', content)
        write_file(b_path, content)

def patch_config_struct():
    path = os.path.join("backend", "config.go")
    content = read_file(path)
    if not content: return
    
    # Inject GitServerConfig struct if not exists
    if "type GitServerConfig struct" not in content:
        git_struct = """
type GitServerConfig struct {
\tName       string `json:"name"`
\tURL        string `json:"url"`
\tSSHKeyPath string `json:"ssh_key_path"`
\tPassword   string `json:"password"`
}
"""
        content = re.sub(r'(type Config struct\s*\{)', git_struct + r'\n\1', content)
    
    # Add array and active flag to Config struct
    if "GitServers" not in content:
        content = re.sub(r'(type Config struct\s*\{)', 
                         r'\1\n\tActiveGitIndex int                `json:"active_git_index"`\n\tGitServers     []GitServerConfig  `json:"git_servers"`', 
                         content)

    # Initialize empty slots in LoadConfig
    if "config.GitServers" not in content and "func LoadConfig" in content:
        init_logic = """
\t// Ensure 5 Git server slots exist
\tfor len(cfg.GitServers) < 5 {
\t\tcfg.GitServers = append(cfg.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(cfg.GitServers)+1)})
\t}
"""
        # Inject right before returning the config
        content = re.sub(r'(return cfg\n\})', init_logic + r'\1', content)

    write_file(path, content)

def patch_git_helper():
    path = os.path.join("backend", "git_helper.go")
    content = read_file(path)
    if not content: return

    # Fix the "Old Value" caching bug by forcing a remote URL update before dialing
    # We look for the repo initialization or plain open
    fix_logic = """
\t// [OMN-Go 1.5.6 Bugfix]: Dynamically update the repo's remote origin so it doesn't use the old cached config!
\tactiveGit := cfg.GitServers[cfg.ActiveGitIndex]
\tremote, err := repo.Remote("origin")
\tif err == nil && len(remote.Config().URLs) > 0 && remote.Config().URLs[0] != activeGit.URL {
\t\trepo.DeleteRemote("origin")
\t\trepo.CreateRemote(&gitconfig.RemoteConfig{
\t\t\tName: "origin",
\t\t\tURLs: []string{activeGit.URL},
\t\t})
\t}
"""
    # Replace standard SSH auth fetching to use the active server index
    if "ActiveGitIndex" not in content:
        # We replace any usage of the old single key with the array lookup
        content = re.sub(r'ssh\.NewPublicKeysFromFile\("git",\s*cfg\.[A-Za-z0-9_]+,\s*""\)', 
                         r'ssh.NewPublicKeysFromFile("git", cfg.GitServers[cfg.ActiveGitIndex].SSHKeyPath, cfg.GitServers[cfg.ActiveGitIndex].Password)', 
                         content)
        
        # Inject the remote URL update logic into the sync/push functions
        # This regex looks for git.PlainOpen and injects the update logic safely below it
        content = re.sub(r'(repo,\s*err\s*:=\s*git\.PlainOpen[^\n]+\n\s*if\s*err\s*!=\s*nil\s*\{[^\}]+\}\n)', 
                         r'\1' + fix_logic, 
                         content)

        # Make sure gitconfig is imported
        if "config" not in content and "github.com/go-git/go-git/v5/config" not in content:
            content = re.sub(r'(import\s*\(\n)', r'\1\tgitconfig "github.com/go-git/go-git/v5/config"\n', content)

    write_file(path, content)

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    if not os.path.exists(path):
        path = os.path.join("backend", "handlers_web.go")
    content = read_file(path)
    if not content: return

    # Inject the multi-server UI into the Config HTML generator
    if "GitServers[" not in content and "ActiveGitIndex" not in content:
        # A robust injection that builds the 5 configuration blocks
        ui_logic = """
\t// Generate Multi-Server UI
\tgitHTML := "<h3>Git Servers</h3>"
\tfor i, gs := range cfg.GitServers {
\t\tchecked := ""
\t\tif cfg.ActiveGitIndex == i {
\t\t\tchecked = "checked"
\t\t}
\t\tgitHTML += fmt.Sprintf(`
\t\t\t<div class="p-4 mb-4 border rounded shadow-sm bg-gray-50">
\t\t\t\t<label class="font-bold flex items-center gap-2">
\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s> Use as Active Server (Slot %d)
\t\t\t\t</label>
\t\t\t\t<div class="grid grid-cols-2 gap-2 mt-2">
\t\t\t\t\t<input type="text" name="git_name_%d" value="%s" placeholder="Server Name" class="border p-1 w-full">
\t\t\t\t\t<input type="text" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" class="border p-1 w-full">
\t\t\t\t\t<input type="text" name="git_ssh_%d" value="%s" placeholder="SSH Key Path" class="border p-1 w-full">
\t\t\t\t\t<input type="password" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" class="border p-1 w-full">
\t\t\t\t</div>
\t\t\t</div>`, i, checked, i+1, i, gs.Name, i, gs.URL, i, gs.SSHKeyPath, i, gs.Password)
\t}
\t// Append it right before the save button
"""
        # Look for the config template generation and inject
        content = re.sub(r'(<button[^>]*>Save Configuration<\/button>)', r'%s\n\t\t\1' % '`+gitHTML+`', content)
        
        # Inject the POST parsing logic
        post_logic = """
\t\t// Parse Git array
\t\tcfg.ActiveGitIndex, _ = strconv.Atoi(r.FormValue("active_git_index"))
\t\tfor i := 0; i < 5; i++ {
\t\t\tcfg.GitServers[i].Name = r.FormValue(fmt.Sprintf("git_name_%d", i))
\t\t\tcfg.GitServers[i].URL = r.FormValue(fmt.Sprintf("git_url_%d", i))
\t\t\tcfg.GitServers[i].SSHKeyPath = r.FormValue(fmt.Sprintf("git_ssh_%d", i))
\t\t\tcfg.GitServers[i].Password = r.FormValue(fmt.Sprintf("git_pass_%d", i))
\t\t}
"""
        # Place POST logic right before writing config.json
        content = re.sub(r'(SaveConfig\(cfg\))', post_logic + r'\n\t\t\1', content)

    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_config_struct()
    patch_git_helper()
    patch_handlers_ui()
    print("[*] Update complete. Rebuild the application to apply the multi-server SSH architecture and remote-cache bugfix.")

if __name__ == "__main__":
    main()