import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.19 Localhost Auth Bypass...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.18"', 'APP_VERSION = "1.2.19"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.18";', 'const APP_VERSION = "1.2.19";'),
        ("backend/frontend/index.html", "let v = '1.2.18';", "let v = '1.2.19';"),
        ("android/app/build.gradle", "versionCode 10218", "versionCode 10219"),
        ("android/app/build.gradle", 'versionName "1.2.18"', 'versionName "1.2.19"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go to inject net imports and the local IP check
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Fix A: Import the "net" package for IP parsing
        if '"net"' not in server_code:
            server_code = server_code.replace('"net/http"', '"net"\n\t"net/http"')
            print("  [+] Imported 'net' package into server.go.")

        # Fix B: Overwrite authMiddleware with the isLocalConnection bypass
        old_auth = '''func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}'''

        new_auth = '''func isLocalConnection(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func authMiddleware(next http.HandlerFunc, requireAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Automatically bypass authorization for internal OS/WebView connections
		if isLocalConnection(r) {
			next(w, r)
			return
		}

		cookie, err := r.Cookie("session_role")
		if err != nil || (requireAdmin && cookie.Value != "admin") || (!requireAdmin && cookie.Value != "admin" && cookie.Value != "guest") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}'''

        if old_auth in server_code:
            server_code = server_code.replace(old_auth, new_auth)
            print("  [+] Upgraded authMiddleware to automatically bypass localhost connections.")
            
        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """feat(security): auto-authorize local connections while securing LAN

- Imported standard library `net` package to securely split incoming IP addresses and ports.
- Injected `isLocalConnection()` into `server.go` to evaluate `r.RemoteAddr`.
- Modified `authMiddleware` to instantly grant administrative bypass to any internal connection (`127.0.0.1`, `::1`, `localhost`).
- External requests spanning from the WiFi/LAN network continue to strictly enforce cookie authorization matching the `config.json` passwords.
- Bumped application to V1.2.19 (Android 10219)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()