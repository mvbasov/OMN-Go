package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		pwd := r.FormValue("password")
		if pwd == appConfig.AdminPassword {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "admin", Path: "/", MaxAge: 86400 * 30})
		} else if pwd == appConfig.GuestPassword {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "guest", Path: "/", MaxAge: 86400 * 30})
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Write([]byte(`
		<html>
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; background: #f4f4f4; margin: 0;">
			<form method="POST" style="background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1);">
				<h2 style="margin-top: 0;">OMN-Go Login</h2>
				<input type="password" name="password" placeholder="Password" style="width: 100%; padding: 10px; margin-bottom: 15px; border: 1px solid #ccc; border-radius: 4px;" autofocus>
				<button type="submit" style="width: 100%; padding: 10px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">Login</button>
			</form>
		</body>
		</html>
	`))
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		appConfig.ServerPort = 8080 // Hardcode or parse if editable
		appConfig.AdminPassword = r.FormValue("admin_password")
		appConfig.GuestPassword = r.FormValue("guest_password")
		appConfig.Author = r.FormValue("author")
		appConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"
		
		cfgPath := filepath.Join(storageDir, "config.json")
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(cfgPath, data, 0644)
		w.Write([]byte("Config saved!"))
		return
	}
}

func getConfigPageBody() []byte {
	htmlStr := string(frontendHTML)
	htmlStr = strings.Replace(htmlStr, "{{TITLE}}", "Config", 1)

	cfgUI := fmt.Sprintf(`
		<div style="max-width: 600px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1);">
			<h2>System Configuration</h2>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Server Port</label>
				<input type="number" id="cfgPort" value="%d" disabled style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box; background: #eee;" />
			</div>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Admin Password</label>
				<input type="text" id="cfgAdminPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>
			
			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Guest Password</label>
				<input type="text" id="cfgGuestPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>

			<div style="margin-bottom: 20px;">
				<label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Author Name</label>
				<input type="text" id="cfgAuthor" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
			</div>
			
			<div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
				<input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />
				<label style="font-weight: 600; color: #444; cursor: pointer;" for="cfgUseInternal">Use Internal Web Editor</label>
			</div>

			<button onclick="saveConfig()" style="width: 100%%; padding: 12px; background: #28a745; color: white; border: none; border-radius: 4px; font-size: 16px; cursor: pointer; font-weight: 600;">Save Configuration</button>
		</div>

		<script>
		async function saveConfig() {
			const params = new URLSearchParams();
			params.append("admin_password", document.getElementById("cfgAdminPwd").value);
			params.append("guest_password", document.getElementById("cfgGuestPwd").value);
			params.append("author", document.getElementById("cfgAuthor").value);
			params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
			
			const res = await fetch('/api/config', { method: 'POST', body: params });
			if (res.ok) {
				alert('Configuration Saved!');
				window.location.reload();
			} else {
				alert('Failed to save configuration.');
			}
		}
		</script>
	`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
		func() string {
			if appConfig.UseInternalEd {
				return "checked"
			}
			return ""
		}())

	htmlStr = strings.Replace(htmlStr, "{{CONTENT}}", cfgUI, 1)
	return []byte(htmlStr)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/Welcome.html"
	}

	if path == "/Config.html" {
		w.Write(getConfigPageBody())
		return
	}

	if strings.HasSuffix(path, ".html") && !strings.HasPrefix(path, "/html/") {
		name := strings.TrimPrefix(strings.TrimSuffix(path, ".html"), "/")
		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
		mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)

		forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
		if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {
				defaultContent := "<!-- OMN_GO_RAW_MD -->\n\n"
				os.MkdirAll(filepath.Dir(mdPath), 0755)
				os.WriteFile(mdPath, []byte(defaultContent), 0644)
			}

			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				if errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime()) {
					humanNameExt := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
					updatedContent := ensureHeaderModified(string(mdContent), humanNameExt)
					if updatedContent != string(mdContent) {
						os.WriteFile(mdPath, []byte(updatedContent), 0644)
						mdContent = []byte(updatedContent)
					}
				}
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}

		content, err := os.ReadFile(htmlPath)
		if err == nil {
			contentStr := string(content)
			startMarker := "<!-- OMN_CONTENT_START -->\n"
			endMarker := "\n<!-- OMN_CONTENT_END -->"
			
			startIdx := strings.Index(contentStr, startMarker)
			endIdx := strings.Index(contentStr, endMarker)
			
			var payload string
			if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
				payload = contentStr[startIdx+len(startMarker) : endIdx]
			} else {
				if mdData, errMd := os.ReadFile(mdPath); errMd == nil {
					newCompiled := compilePage(name, mdData)
					os.WriteFile(htmlPath, newCompiled, 0644)
					contentStr = string(newCompiled)
					startIdx = strings.Index(contentStr, startMarker)
					endIdx = strings.Index(contentStr, endMarker)
					if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
						payload = contentStr[startIdx+len(startMarker) : endIdx]
					} else {
						payload = contentStr
					}
				} else {
					payload = contentStr
				}
			}

			appShell := string(frontendHTML)
			humanName := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
			appShell = strings.Replace(appShell, "{{TITLE}}", humanName, 1)
			appShell = strings.Replace(appShell, "{{CONTENT}}", payload, 1)
			
			if !appConfig.UseInternalEd {
				appShell = strings.Replace(appShell, `id="toggleBtn"`, `id="toggleBtn" style="display:none;"`, 1)
			}
			
			w.Write([]byte(appShell))
			return
		}
	}

	serveLazyEmbed(w, r)
}
