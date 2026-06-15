import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.44"', 'APP_VERSION = "1.0.45"'),
        ("frontend/index.html", 'APP_VERSION = "1.0.44"', 'APP_VERSION = "1.0.45"')
    ]

    for filepath, old_v, new_v in version_replacements:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))
            # Catch scenario where 1.0.44 version bump was skipped or failed
            elif 'APP_VERSION = "1.0.43"' in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace('APP_VERSION = "1.0.43"', new_v))

    # 2. Targeted Clean-Up for server.go Duplication Bug
    server_go_path = "backend/server.go" if os.path.exists("backend/server.go") else "server.go"
    if os.path.exists(server_go_path):
        with open(server_go_path, 'r', encoding='utf-8') as f:
            content = f.read()

        original_newlines = "\r\n" if "\r\n" in content else "\n"
        normalized_content = content.replace("\r\n", "\n")

        exact_injected_block = r'''
func handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}
	defer file.Close()

	jsonDir := filepath.Join(storageDir, "user_json")
	os.MkdirAll(jsonDir, 0755)
	
	destPath := filepath.Join(jsonDir, header.Filename)
	dest, _ := os.Create(destPath)
	defer dest.Close()
	io.Copy(dest, file)
	
	w.Write([]byte(fmt.Sprintf("[%s]({filename}/user_json/%s)", header.Filename, header.Filename)))
}'''

        # Step A: Strip ALL occurrences of handleUploadJSON completely (Literal & Regex Fallback)
        cleaned_norm = normalized_content.replace(exact_injected_block, "")
        cleaned_norm = cleaned_norm.replace("\n" + exact_injected_block, "")
        
        block_regex = r'\n*func handleUploadJSON\(w http\.ResponseWriter, r \*http\.Request\) \{[\s\S]*?user_json[\s\S]*?\n\}'
        cleaned_norm = re.sub(block_regex, '', cleaned_norm)

        # Step B: Add it back EXACTLY ONCE right after handleUpload
        anchor_norm = r'''	w.Write([]byte(fmt.Sprintf("![%s]({filename}/images/%s)", header.Filename, header.Filename)))
}'''
        
        if anchor_norm in cleaned_norm:
            cleaned_norm = cleaned_norm.replace(anchor_norm, anchor_norm + "\n" + exact_injected_block)
        else:
            print("Warning: Could not find handleUpload hook to inject handleUploadJSON.")

        # Restore original newlines
        final_content = cleaned_norm.replace("\n", original_newlines)

        with open(server_go_path, 'w', encoding='utf-8') as f:
            f.write(final_content)
        print(f"Successfully deduplicated handleUploadJSON in {server_go_path}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """fix(backend): resolve handleUploadJSON redeclaration compiler error
    
- Implement targeted regex cleaner to wipe duplicated `handleUploadJSON` definitions.
- Restructure function patching to guarantee idempotency.

Version bumped to 1.0.45"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()