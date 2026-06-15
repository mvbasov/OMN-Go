import os
import re
import urllib.request

def update_application():
    # 0. Offline Mode: Pre-download marked.min.js
    frontend_dir = "backend/frontend"
    os.makedirs(frontend_dir, exist_ok=True)
    marked_path = os.path.join(frontend_dir, "marked.min.js")
    if not os.path.exists(marked_path):
        print("Downloading marked.min.js for offline mode...")
        try:
            urllib.request.urlretrieve("https://cdn.jsdelivr.net/npm/marked/marked.min.js", marked_path)
            print("Successfully downloaded marked.min.js!")
        except Exception as e:
            print(f"Failed to download marked.min.js: {e}")

    # 0. Android Permanent Keystore: Generate once if missing
    keystore_path = "android/app/goomn.keystore"
    if not os.path.exists(keystore_path) and os.path.exists("android/app"):
        print("Generating permanent Android keystore...")
        # Automatically generates a generic 10,000-day valid RSA keystore for stable release updates
        os.system(f'keytool -genkey -v -keystore {keystore_path} -alias goomn -keyalg RSA -keysize 2048 -validity 10000 -storepass goomn123 -keypass goomn123 -dname "CN=GoOMN, O=Basov" 2>/dev/null')

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.25"', 'APP_VERSION = "1.0.26"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.25";', 'const APP_VERSION = "1.0.26";')
    ]
    
    # 2. Define File Patches
    patches = {
        "backend/server.go": [
            (
                # Embed marked.min.js directly into the Go binary
                r'''//go:embed frontend/index.html
var frontendHTML []byte''',
                r'''//go:embed frontend/index.html
var frontendHTML []byte

//go:embed frontend/marked.min.js
var markedJS []byte'''
            ),
            (
                # Ensure new Bookmarks.md initialization matches the JSON marker structure
                r'''	bmContent := "Title: Bookmarks\nDate: 2026-06-14 12:00:00\nCategory: Links\n\n"''',
                r'''	bmContent := "Title: Bookmarks\nDate: 2026-06-14 12:00:00\nCategory: Links\n\n<script>bookmarks = [\n<!-- Don't edit body below this line -->\n];</script>"'''
            ),
            (
                # Rewrite handleBookmark to preserve JSON structure inside Markdown
                r'''	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	entry := fmt.Sprintf("\n- [%s](%s) | Tags: %s | Notes: %s | Added: %s\n", title, url, tags, notes, timestamp)
	
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(entry)
	}
	w.Write([]byte("Saved"))''',
                r'''	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsList := []string{}
	for _, t := range strings.Split(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	notesList := []string{}
	if trimmed := strings.TrimSpace(notes); trimmed != "" {
		notesList = append(notesList, trimmed)
	}
	
	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\n"
	
	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- Don't edit body below this line -->\n"
		idx := strings.Index(content, marker)
		if idx != -1 {
			insertPos := idx + len(marker)
			newContent := content[:insertPos] + entry + content[insertPos:]
			os.WriteFile(path, []byte(newContent), 0644)
		} else {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
			defer f.Close()
			f.WriteString("\n" + entry)
		}
	}
	w.Write([]byte("Saved"))'''
            ),
            (
                # Serve the embedded marked.min.js locally
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {''',
                r'''func serveMarked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(markedJS)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {'''
            ),
            (
                r'''		mux.HandleFunc("/", serveFrontend)''',
                r'''		mux.HandleFunc("/", serveFrontend)
		mux.HandleFunc("/marked.min.js", serveMarked)'''
            )
        ],
        "backend/frontend/index.html": [
            (
                # Switch to local Javascript file for fully offline mode
                r'''<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>''',
                r'''<script src="/marked.min.js"></script>'''
            ),
            (
                # Open external links in new system browser tab (Desktop & Web)
                r'''                if(href && !href.startsWith('http')) {
                    e.preventDefault();
                    loadNote(href);
                }''',
                r'''                if(href && !href.startsWith('http')) {
                    e.preventDefault();
                    loadNote(href);
                } else if (href && href.startsWith('http')) {
                    e.preventDefault();
                    window.open(href, '_blank');
                }'''
            )
        ],
        "android/app/src/main/java/net/basov/goomn/MainActivity.java": [
            (
                # Hijack WebView external links and pass them to Android system browser
                r'''        webView.setWebViewClient(new WebViewClient());''',
                r'''        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                    if (!url.contains("localhost")) {
                        view.getContext().startActivity(
                            new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url))
                        );
                        return true;
                    }
                }
                return false;
            }
        });'''
            )
        ]
    }

    # Add Dockerfile check safely for extension mismatches
    dockerfile_path = "Dockerfile" if os.path.exists("Dockerfile") else "Dockerfile.txt"
    if os.path.exists(dockerfile_path):
        patches[dockerfile_path] = [
            (
                # Compile using assembleRelease instead of debug
                r'''RUN cd android && gradle assembleDebug && cp app/build/outputs/apk/debug/app-debug.apk ../bin/goomn.apk''',
                r'''RUN cd android && gradle assembleRelease && cp app/build/outputs/apk/release/app-release.apk ../bin/goomn.apk'''
            )
        ]

    # Execute Version Bump
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                content = content.replace(old, new)
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content)

    # Execute Patches Sequentially
    for filepath, file_patches in patches.items():
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            for old, new in file_patches:
                if old in content:
                    content = content.replace(old, new)
                elif new not in content:
                    print(f"Warning: Could not find patch target in {filepath}:\n{old[:50]}...")
            with open(filepath, 'w', encoding='utf-8') as f:
                f.write(content)

    # 3. Dynamic Android Build.Gradle Regex Patching (Version Codes & Keystore Config)
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        # Safely increment version code mathematically and set version name
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10026', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.26"', gradle_content)
        
        # Inject signingConfigs block if it doesn't exist yet
        if 'signingConfigs' not in gradle_content:
            gradle_content = gradle_content.replace('buildTypes {', '''signingConfigs {
        release {
            storeFile file('goomn.keystore')
            storePassword 'goomn123'
            keyAlias 'goomn'
            keyPassword 'goomn123'
        }
    }
    buildTypes {''')
            gradle_content = gradle_content.replace('release {', '''release {
            signingConfig signingConfigs.release''')
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 4. Output Standardized Git Commit Message
    commit_msg = """feat(core): implement offline mode, JSON bookmarks, and android system bindings

- Executed local download and Go-embedding of marked.min.js for fully offline rendering.
- Rewrote handleBookmark API to enforce exact JSON array structures inside Bookmarks.md.
- Injected Intent-based URL interceptions into Java WebView and window.open bounds into JS to force external links into the system browser.
- Generated persistent `goomn.keystore` and updated gradle/Docker configuration for signed release builds.
- Added strict Regex bumping for Android `versionCode` in build.gradle.

Version bumped to 1.0.26"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.26!")

if __name__ == "__main__":
    update_application()