import os
import re

def apply_patch(filepath, old_str, new_str, description):
    print(f"\n[PATCH] {description}")
    print(f"  Target: {filepath}")
    
    if not os.path.exists(filepath):
        print(f"  [-] ERROR: File {filepath} not found!")
        return False
        
    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()
        
    if new_str in content:
        print("  [+] SUCCESS: Patch appears to be already applied from a previous run.")
        return True
        
    if old_str in content:
        content = content.replace(old_str, new_str)
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)
        print("  [+] SUCCESS: Exact string match replaced.")
        return True
        
    old_normalized = old_str.replace('\r\n', '\n')
    content_normalized = content.replace('\r\n', '\n')
    
    if old_normalized in content_normalized:
        content_normalized = content_normalized.replace(old_normalized, new_str.replace('\r\n', '\n'))
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content_normalized)
        print("  [+] SUCCESS: Normalized match replaced.")
        return True
        
    print("  [-] ERROR: Target string NOT FOUND!")
    return False

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.4.3")
    
    # Note: Since the refactoring, APP_VERSION is now housed in backend/config.go
    versions = [
        ("backend/config.go", 'APP_VERSION = "1.4.2"', 'APP_VERSION = "1.4.3"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.4.2";', 'const APP_VERSION = "1.4.3";'),
        ("android/app/build.gradle", 'versionCode 10402', 'versionCode 10403'),
        ("android/app/build.gradle", 'versionName "1.4.2"', 'versionName "1.4.3"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10403', content)
                    content = re.sub(r'versionName\s+"1\.4\.\d+"', 'versionName "1.4.3"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.4\.\d+"', 'APP_VERSION = "1.4.3"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")
        else:
            print(f"  [-] Skipped {fp} (File not found)")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.4.3)")
    print("==================================================")
    
    bump_versions()

    # 1. API Handlers: handleNewPage
    old_newpage = (
        '\tnow := time.Now().Format("2006-01-02 15:04:05")\n\n'
        '\ttargetMdPath := filepath.Join(storageDir, "md", target+".md")\n'
        '\tif _, err := os.Stat(targetMdPath); os.IsNotExist(err) {\n'
        '\t\tauthorLine := ""\n'
        '\t\tif appConfig.Author != "" {\n'
        '\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t}\n'
        '\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s\\nCategory: Notes%s\\n\\n", title, now, now, authorLine)'
    )
    new_newpage = (
        '\ttargetMdPath := filepath.Join(storageDir, "md", target+".md")\n'
        '\tif _, err := os.Stat(targetMdPath); os.IsNotExist(err) {\n'
        '\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    )
    apply_patch("backend/handlers_api.go", old_newpage, new_newpage, "Replace Pelican header generation with Raw MD tag in handleNewPage")

    # 2. API Handlers: handleGetNote
    old_getnote = (
        '\t\thumanTitle := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")\n'
        '\t\ttimestamp := time.Now().Format("2006-01-02 15:04:05")\n'
        '\t\tauthorLine := ""\n'
        '\t\tif appConfig.Author != "" {\n'
        '\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t}\n'
        '\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", humanTitle, timestamp, authorLine)'
    )
    new_getnote = '\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    apply_patch("backend/handlers_api.go", old_getnote, new_getnote, "Replace Pelican header generation with Raw MD tag in handleGetNote")

    # 3. Web Handlers: serveFrontend
    old_serve = (
        '\t\t\t\ttimestamp := time.Now().Format("2006-01-02 15:04:05")\n'
        '\t\t\t\tauthorLine := ""\n'
        '\t\t\t\tif appConfig.Author != "" {\n'
        '\t\t\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t\t\t}\n'
        '\t\t\t\thumanName := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")\n'
        '\t\t\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", humanName, timestamp, authorLine)'
    )
    new_serve = '\t\t\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    apply_patch("backend/handlers_web.go", old_serve, new_serve, "Replace Pelican header generation with Raw MD tag in serveFrontend")

    # 4. Markdown Logic: ensureHeaderModified fallback
    old_markdown = (
        '\tauthorLine := ""\n'
        '\tif appConfig.Author != "" {\n'
        '\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t}\n'
        '\treturn fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s%s\\n\\n%s", defaultTitle, now, now, authorLine, content)'
    )
    new_markdown = (
        '\tif strings.HasPrefix(strings.TrimSpace(content), "<!-- OMN_GO_RAW_MD -->") {\n'
        '\t\treturn content\n'
        '\t}\n'
        '\treturn fmt.Sprintf("<!-- OMN_GO_RAW_MD -->\\n\\n%s", content)'
    )
    apply_patch("backend/markdown.go", old_markdown, new_markdown, "Update ensureHeaderModified fallback to prepend Raw MD tag instead of Pelican block")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "feat(core): replace default Pelican headers with raw markdown markers for new pages\n\nVersion bumped to 1.4.3"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()