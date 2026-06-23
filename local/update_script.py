#!/usr/bin/env python3
import os

def update_application():
    """OMN-Go 1.3.25 → 1.3.26: enable ?edit=true for .js/.css/.json files."""

    # ----- Version bumps -----
    version_patches = [
        ("backend/version.go",
         'APP_VERSION = "1.3.25"',
         'APP_VERSION = "1.3.26"'),
        ("android/app/build.gradle",
         'versionCode 10325',
         'versionCode 10326'),
        ("android/app/build.gradle",
         'versionName "1.3.25"',
         'versionName "1.3.26"'),
    ]
    for path, old, new in version_patches:
        with open(path, "r", encoding="utf-8") as f:
            text = f.read()
        if old not in text:
            raise ValueError(f"Missing string in {path}:\n{old}")
        text = text.replace(old, new)
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)

    # ----- Feature patches in handlers.go -----
    target = "backend/handlers.go"
    with open(target, "r", encoding="utf-8") as f:
        code = f.read()

    # Patch 1: intercept ?edit=true before the static file fallback
    anchor1 = "\t// Unified Content-Type Resolver based strictly on extension"
    if anchor1 not in code:
        raise ValueError("Patch 1 anchor not found")
    insertion1 = (
        '\t// Serve editor for any file when ?edit=true\n'
        '\tif r.URL.Query().Get("edit") == "true" {\n'
        '\t\trelPath := strings.TrimPrefix(r.URL.Path, "/")\n'
        '\t\tvar filePath string\n'
        '\t\tvar rawContent []byte\n'
        '\t\tif strings.HasSuffix(relPath, ".md") {\n'
        '\t\t\tfilePath = filepath.Join(storageDir, "md", filepath.Clean(relPath))\n'
        '\t\t} else {\n'
        '\t\t\tfilePath = filepath.Join(storageDir, "html", filepath.Clean(relPath))\n'
        '\t\t}\n'
        '\t\tif data, err := os.ReadFile(filePath); err == nil {\n'
        '\t\t\trawContent = data\n'
        '\t\t}\n'
        '\t\t// Show raw content in preview, leave editor empty – loaded on demand via API\n'
        '\t\tescapedContent := htmlEscape(string(rawContent))\n'
        '\t\tcustomBody := "<pre style=\\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\\">" + escapedContent + "</pre>"\n'
        '\t\tcompiled := compilePageWithBody(relPath, []byte{}, customBody)\n'
        '\t\t// Tell the frontend this is not a Pelican markdown page\n'
        '\t\tscriptInjection := "<script>var IS_MARKDOWN = false;</script>"\n'
        '\t\tcompiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\\n</head>", 1))\n'
        '\t\tw.Header().Set("Content-Type", "text/html")\n'
        '\t\tw.Write(compiled)\n'
        '\t\treturn\n'
        '\t}\n\n'
        '\t// Unified Content-Type Resolver based strictly on extension'
    )
    code = code.replace(anchor1, insertion1)

    # Patch 2: modernise getExternalEditPageBody (full filename, no forced .md)
    anchor2 = (
        'func getExternalEditPageBody(name string) string {\n'
        '\treturn fmt.Sprintf(`\n'
        '<div style="max-width: 600px; margin: 40px auto; background: #ffffff; padding: 40px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8; text-align: center;">\n'
        '    <div style="font-size: 48px; margin-bottom: 20px;">📝</div>\n'
        '    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700;">Editing Externally</h2>\n'
        '    <p style="color: #555; font-size: 16px; margin-bottom: 30px; line-height: 1.5;">\n'
        '        We have launched <strong>%s</strong> to edit <code>%s.md</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated page.\n'
        '    </p>\n'
        '    <button onclick="window.location.replace(\'/%s.html\')" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">\n'
        '        Press after edit to refresh view\n'
        '    </button>\n'
        '</div>\n'
        '`, appConfig.DesktopExtCmd, name, name)\n'
        '}'
    )
    if anchor2 not in code:
        raise ValueError("Patch 2 anchor not found")
    replacement2 = (
        'func getExternalEditPageBody(fileName string) string {\n'
        '\treturn fmt.Sprintf(`\n'
        '<div style="max-width: 600px; margin: 40px auto; background: #ffffff; padding: 40px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8; text-align: center;">\n'
        '    <div style="font-size: 48px; margin-bottom: 20px;">📝</div>\n'
        '    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700;">Editing Externally</h2>\n'
        '    <p style="color: #555; font-size: 16px; margin-bottom: 30px; line-height: 1.5;">\n'
        '        We have launched <strong>%s</strong> to edit <code>%s</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated file.\n'
        '    </p>\n'
        '    <button onclick="window.location.replace(\'/%s\')" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">\n'
        '        Press after edit to refresh view\n'
        '    </button>\n'
        '</div>\n'
        '`, appConfig.DesktopExtCmd, fileName, fileName)\n'
        '}'
    )
    code = code.replace(anchor2, replacement2)

    # Patch 3: fix handleEditExternal file path construction
    anchor3 = (
        '\tcleanName := strings.TrimSuffix(name, ".html")\n'
        '\tif !strings.HasSuffix(cleanName, ".md") {\n'
        '\t\tcleanName += ".md"\n'
        '\t}\n'
        '\tfilePath := filepath.Join(storageDir, "md", cleanName)'
    )
    if anchor3 not in code:
        raise ValueError("Patch 3 anchor not found")
    replacement3 = (
        '\tvar filePath string\n'
        '\tif strings.HasSuffix(name, ".md") {\n'
        '\t\tfilePath = filepath.Join(storageDir, "md", filepath.Clean(name))\n'
        '\t} else {\n'
        '\t\tfilePath = filepath.Join(storageDir, "html", filepath.Clean(name))\n'
        '\t}'
    )
    code = code.replace(anchor3, replacement3)

    # Patch 4: remove now-unused pageName variable in handleEditExternal
    anchor4 = (
        '\tpageName := strings.TrimSuffix(cleanName, ".md")\n'
        '\twaitBody := getExternalEditPageBody(pageName)\n'
        '\tcompiledWait := compilePageWithBody(pageName, fmt.Appendf(nil, "Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", pageName, time.Now().Format("2006-01-02 15:04:05")), waitBody)'
    )
    if anchor4 not in code:
        raise ValueError("Patch 4 anchor not found")
    replacement4 = (
        '\twaitBody := getExternalEditPageBody(name)\n'
        '\tcompiledWait := compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)'
    )
    code = code.replace(anchor4, replacement4)

    with open(target, "w", encoding="utf-8") as f:
        f.write(code)

    # ----- Commit message -----
    commit = (
        "feat(core): enable editing of .js, .css, .json via ?edit=true\n\n"
        "Allow any non-HTML file to be opened in the internal editor by appending\n"
        "?edit=true to its URL.  The editor shows the raw content without Pelican\n"
        "header injection or markdown compilation.  The raw file is still served\n"
        "normally when accessed without the query parameter.\n\n"
        "- New edit handler in serveFrontend intercepts ?edit=true for all file\n"
        "  types and serves the editor UI with raw content preview.\n"
        "- External editor (DesktopExtCmd) now correctly locates .js/.css/.json\n"
        "  files inside the html/ directory and links back to the original URL.\n"
        "- getExternalEditPageBody signature changed to accept the full filename.\n"
        "- handleEditExternal builds the correct file path based on extension.\n\n"
        "Version bumped to 1.3.26"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()