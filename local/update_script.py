import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.28...")

def bump_version():
    # 1. Update backend/config.go
    cfg_path = "backend/config.go"
    if os.path.exists(cfg_path):
        with open(cfg_path, "r") as f:
            c = f.read()
        c = c.replace('Version = "1.4.27"', 'Version = "1.4.28"')
        with open(cfg_path, "w") as f:
            f.write(c)
        print("  [+] Bumped version in backend/config.go")

    # 2. Update frontend
    html_path = "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, "r") as f:
            c = f.read()
        c = c.replace('1.4.27', '1.4.28')
        with open(html_path, "w") as f:
            f.write(c)
        print("  [+] Bumped version in backend/frontend/index.html")

    # 3. Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f:
            c = f.read()
        c = c.replace('versionCode 10427', 'versionCode 10428')
        c = c.replace('versionName "1.4.27"', 'versionName "1.4.28"')
        with open(gradle_path, "w") as f:
            f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def patch_git_ssh():
    # Fix existing SSH authentications
    patched = False
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f:
            content = f.read()

        if "ssh.NewPublicKeysFromFile" in content and "InsecureIgnoreHostKey" not in content:
            print(f"  [*] Patching SSH HostKey restriction in {go_file}")
            # Add crypto/ssh import if missing
            if "golang.org/x/crypto/ssh" not in content:
                content = re.sub(r'import\s+\(', 'import (\n\tgossh "golang.org/x/crypto/ssh"\n', content, count=1)

            # Inject the insecure bypass right after the err check
            content = re.sub(
                r'(?P<var>[A-Za-z0-9_]+),\s*(?P<err>[A-Za-z0-9_]+)\s*:=\s*ssh\.NewPublicKeysFromFile[^\n]+\n\s*if\s*(?P=err)\s*!=\s*nil\s*\{[^\}]+\}',
                lambda m: m.group(0) + f'\n\t{m.group("var")}.HostKeyCallback = gossh.InsecureIgnoreHostKey()',
                content,
                flags=re.DOTALL
            )
            with open(go_file, "w") as f:
                f.write(content)
            patched = True

    # Generate a helper to be 100% safe
    helper_code = """package backend

import (
\t"os"
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
\t// CRITICAL FIX: Ignore host key verification for gitolite3 servers
\tpublicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
\treturn publicKeys, nil
}
"""
    with open("backend/git_helper.go", "w") as f:
        f.write(helper_code)
    print("  [+] Generated robust SSH helper in backend/git_helper.go")

def inject_go_logger():
    # 1. Create the SSE logger
    logger_code = """package backend

import (
\t"fmt"
\t"log"
\t"net/http"
\t"sync"
)

var (
\tlogMutex   sync.Mutex
\tlogClients []chan string
)

type JSLogger struct{}

func (l *JSLogger) Write(p []byte) (n int, err error) {
\tmsg := string(p)
\tlogMutex.Lock()
\tfor _, c := range logClients {
\t\tselect {
\t\tcase c <- msg:
\t\tdefault:
\t\t}
\t}
\tlogMutex.Unlock()
\tfmt.Print(msg)
\treturn len(p), nil
}

func InitLoggerAndRoute() {
\tlog.SetOutput(&JSLogger{})
\thttp.HandleFunc("/api/logs", HandleLogsSSE)
}

func HandleLogsSSE(w http.ResponseWriter, r *http.Request) {
\tw.Header().Set("Content-Type", "text/event-stream")
\tw.Header().Set("Cache-Control", "no-cache")
\tw.Header().Set("Connection", "keep-alive")

\tch := make(chan string, 10)
\tlogMutex.Lock()
\tlogClients = append(logClients, ch)
\tlogMutex.Unlock()

\tdefer func() {
\t\tlogMutex.Lock()
\t\tfor i, c := range logClients {
\t\t\tif c == ch {
\t\t\t\tlogClients = append(logClients[:i], logClients[i+1:]...)
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tlogMutex.Unlock()
\t}()

\tflusher, ok := w.(http.Flusher)
\tif !ok {
\t\treturn
\t}

\tfor {
\t\tselect {
\t\tcase msg := <-ch:
\t\t\tfmt.Fprintf(w, "data: %s\\n\\n", msg)
\t\t\tflusher.Flush()
\t\tcase <-r.Context().Done():
\t\t\treturn
\t\t}
\t}
}
"""
    with open("backend/logger.go", "w") as f:
        f.write(logger_code)
    print("  [+] Created backend/logger.go for SSE log streaming")

    # 2. Inject initialization into the first HTTP handler setup found
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f:
            content = f.read()
        if "http.HandleFunc(" in content and "InitLoggerAndRoute()" not in content:
            content = re.sub(r'(http\.HandleFunc\()', r'InitLoggerAndRoute()\n\t\1', content, count=1)
            with open(go_file, "w") as f:
                f.write(content)
            print(f"  [+] Injected Log Router into {go_file}")
            break

    # 3. Patch index.html to catch the logs
    html_path = "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, "r") as f:
            content = f.read()
        if "/api/logs" not in content:
            log_js = """
    <script>
        // GoOMN Log Interceptor - Bridges Go background logs to JS UI
        document.addEventListener('DOMContentLoaded', () => {
            try {
                const logSource = new EventSource('/api/logs');
                logSource.onmessage = function(event) {
                    let msg = event.data.trim();
                    if(msg) {
                        console.log("[GO] " + msg);
                    }
                };
            } catch(e) {
                console.error("Log source error:", e);
            }
        });
    </script>
"""
            content = content.replace("</body>", log_js + "</body>")
            with open(html_path, "w") as f:
                f.write(content)
            print("  [+] JS Log Interceptor connected in frontend")

def patch_android_manifest():
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r") as f:
            content = f.read()
        if "android.permission.INTERNET" not in content:
            content = content.replace("<application", '<uses-permission android:name="android.permission.INTERNET" />\n    <application')
            with open(manifest_path, "w") as f:
                f.write(content)
            print("  [+] Added missing INTERNET permission for Git Sync in AndroidManifest.xml")

if __name__ == "__main__":
    bump_version()
    patch_git_ssh()
    inject_go_logger()
    patch_android_manifest()
    print("[*] Update complete! Version 1.4.28 ready for compilation.")