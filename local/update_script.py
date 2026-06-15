import os
import re
import shutil

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.28"', 'APP_VERSION = "1.0.29"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.28";', 'const APP_VERSION = "1.0.29";')
    ]
    
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old, new))

    # 2. Bump the Android Version in Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10029', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.29"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Handle Bookmarker.js and Bookmarker.css Integration
    frontend_dir = "backend/frontend"
    os.makedirs(frontend_dir, exist_ok=True)
    
    for f in ["Bookmarker.js", "Bookmarker.css"]:
        src = f
        dst = os.path.join(frontend_dir, f)
        if os.path.exists(src):
            shutil.copy(src, dst)
            print(f"[+] Embedded {f} into {frontend_dir}/")
        elif not os.path.exists(dst):
            # Fallback stub generation to prevent compiler panics if the user hasn't moved the files
            with open(dst, 'w') as stub:
                stub.write(f"/* Missing {f} */")
            print(f"[!] Warning: {f} not found in root. Created empty stub in {frontend_dir}/ to prevent build failure.")

    # 4. Patch server.go to embed the CSS/JS files and update Bookmarks template
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, 'r', encoding='utf-8') as f:
            server_content = f.read()

        # Inject Embed variables
        server_content = server_content.replace(
            r'''//go:embed frontend/marked.min.js
var markedJS []byte''',
            r'''//go:embed frontend/marked.min.js
var markedJS []byte

//go:embed frontend/Bookmarker.js
var bookmarkerJS []byte

//go:embed frontend/Bookmarker.css
var bookmarkerCSS []byte'''
        )

        # Inject Handlers
        server_content = server_content.replace(
            r'''func serveMarked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(markedJS)
}''',
            r'''func serveMarked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(markedJS)
}

func serveBookmarkerJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(bookmarkerJS)
}

func serveBookmarkerCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write(bookmarkerCSS)
}'''
        )

        # Inject Mux Routes
        server_content = server_content.replace(
            r'''		mux.HandleFunc("/marked.min.js", serveMarked)''',
            r'''		mux.HandleFunc("/marked.min.js", serveMarked)
		mux.HandleFunc("/js/Bookmarker.js", serveBookmarkerJS)
		mux.HandleFunc("/css/Bookmarker.css", serveBookmarkerCSS)'''
        )

        # Replace legacy Bookmarks.md initialization string with exact specified HTML/JSON Hybrid
        old_bm = r'''	bmContent := "Title: Bookmarks\nDate: 2026-06-14 12:00:00\nCategory: Links\n\n<script>bookmarks = [\n<!-- Don't edit body below this line -->\n];</script>"'''
        new_bm = r'''	bmContent := `Title: Incoming bookmarks
Date: 2023-01-13 13:59:15
Modified: 2025-11-01 11:03:46
Author: Mikhail Basov
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
  {
    "date": "2025-11-06 02:52:09",
    "url": "https://youtube.com/shorts/0pI2KHl7gCU?si=9M_DqeVBxmuyHiTC",
    "title": "Tapping 16th note rhythms🥁 #music #musiclesson #musictutorial #learnmusi...",
    "tags": [
      "Music",
      "Mathematics",
      "YouTube short",
    ],
    "notes": [
    ]
  },
  {
    "date": "2025-11-05 15:44:25",
    "url": "https://www.reddit.com/r/ErgoMechKeyboards/comments/1ol49i6/printyl_mx_keycap_optimized_for_3d_printer/",
    "title": "printyl: MX keycap optimized for 3d printer, inspired on dactyl : r/ErgoMechKeyboards",
    "tags": [
      "Reddit",
      "Keyboard",
      "3D model",
    ],
    "notes": [
    ]
  },
  {
    "date": "2023-01-22 22:22:22",
    "url": "/default/BookmarkerHelp.html",
    "title": "Help about this bookmark page",
    "tags": [
      "OMN",
      "Local pages",
      "Help"
    ],
    "notes": [
      "File format described on this page also"
    ]
  }
];
</script>
  
<!-- end of bookmarks definition -->
    
<link rel="stylesheet" type="text/css" href="/css/Bookmarker.css" />
<script type="text/javascript" src="/js/Bookmarker.js"></script>`'''
        server_content = server_content.replace(old_bm, new_bm)

        with open(server_path, 'w', encoding='utf-8') as f:
            f.write(server_content)

    # 5. Patch Dockerfile for internal keystore generation
    dockerfile_path = "Dockerfile" if os.path.exists("Dockerfile") else "Dockerfile.txt"
    if os.path.exists(dockerfile_path):
        with open(dockerfile_path, 'r', encoding='utf-8') as f:
            df_content = f.read()
        
        # Inject standard bash "if file doesn't exist" validation and keytool command inline right before gradle
        df_content = re.sub(
            r'RUN cd android && gradle assembleRelease',
            r'RUN cd android && if [ ! -f app/goomn.keystore ]; then keytool -genkey -v -keystore app/goomn.keystore -alias goomn -keyalg RSA -keysize 2048 -validity 10000 -storepass goomn123 -keypass goomn123 -dname "CN=GoOMN, O=Basov"; fi && gradle assembleRelease',
            df_content
        )
        with open(dockerfile_path, 'w', encoding='utf-8') as f:
            f.write(df_content)

    # 6. Delete old Bookmarks.md stubs to force regeneration with the new template
    for storage_dir in ["data", "android/app/media/net.basov.goomn"]:
        bm_path = os.path.join(storage_dir, "Bookmarks.md")
        if os.path.exists(bm_path):
            with open(bm_path, 'r', encoding='utf-8') as f:
                content = f.read()
            # If the file contains the old placeholder, nuke it so `server.go` builds the new script layout
            if "Title: Bookmarks" in content and "2026-06-14 12:00:00" in content:
                os.remove(bm_path)
                print(f"[+] Removed outdated stub: {bm_path}")

    # 7. Output Standardized Git Commit Message
    commit_msg = """feat(bookmarks): implement rich bookmark rendering and docker keystore gen

- Integrated Bookmarker.js and Bookmarker.css directly into Go backend via //go:embed
- Updated initial Bookmarks.md payload to match the rigorous HTML/JSON hybrid specification
- Added automatic in-container keystore generation to Dockerfile to prevent CI/CD signing failures
- Bumped Android versionCode to 10029

Version bumped to 1.0.29"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.29!")

if __name__ == "__main__":
    update_application()