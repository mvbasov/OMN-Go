package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type GitServerConfig struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SSHKeyData string `json:"ssh_key_data"`
	Password   string `json:"password"`
}

type Config struct {
	ForcePullOneTime bool              `json:"force_pull_one_time"`
	ServerPort       int               `json:"server_port"`
	AdminPassword    string            `json:"admin_password"`
	GuestPassword    string            `json:"guest_password"`
	Author           string            `json:"author"`
	UseInternalEd    bool              `json:"use_internal_editor"`
	DesktopExtCmd    string            `json:"desktop_ext_cmd"`
	MimeTypes        map[string]string `json:"mime_types"`
	ActiveGitIndex   int               `json:"active_git_index"`
	GitServers       []GitServerConfig `json:"git_servers"`
}

func (a *App) loadConfig(storageDir string) {
	configPath := filepath.Join(a.StorageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		a.Config = Config{
			ServerPort:    8080,
			AdminPassword: "admin_secret_changeme",
			GuestPassword: "guest_secret_changeme",
			Author:        "Anonymous",
			UseInternalEd: true,
			DesktopExtCmd: "subl",
			MimeTypes: map[string]string{
				".css":   "text/css",
				".js":    "application/javascript",
				".json":  "application/json",
				".html":  "text/html",
				".md":    "text/markdown",
				".svg":   "image/svg+xml",
				".png":   "image/png",
				".jpg":   "image/jpeg",
				".jpeg":  "image/jpeg",
				".woff2": "font/woff2",
			},
		}
		data, _ := json.MarshalIndent(a.Config, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &a.Config)
		// [OMN-Go 1.5.21] Absolute Array Lock: Prevents the JSON 'null' wipe bug forever
		for len(a.Config.GitServers) < 5 {
			a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
		}

	}
	if a.Config.ServerPort == 0 {
		a.Config.ServerPort = 8080
	}
	// [OMN-Go 1.5.16] Enforce 5 empty slots natively
	for len(a.Config.GitServers) < 5 {
		a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
	}

	if a.Config.MimeTypes == nil {
		a.Config.MimeTypes = map[string]string{
			".css":   "text/css",
			".js":    "application/javascript",
			".json":  "application/json",
			".woff2": "font/woff2",
		}
		data, _ := json.MarshalIndent(a.Config, "", "  ")
		os.WriteFile(configPath, data, 0644)
	}

}

