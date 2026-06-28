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
	SSHKeyPath string `json:"ssh_key_path"`
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

var appConfig Config

func loadConfig(storageDir string) {
	configPath := filepath.Join(storageDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		appConfig = Config{
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
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	} else {
		data, _ := os.ReadFile(configPath)
		json.Unmarshal(data, &appConfig)
		// [OMN-Go 1.5.21] Absolute Array Lock: Prevents the JSON 'null' wipe bug forever
		for len(appConfig.GitServers) < 5 {
			appConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
		}

	}
	if appConfig.ServerPort == 0 {
		appConfig.ServerPort = 8080
	}
	// [OMN-Go 1.5.16] Enforce 5 empty slots natively
	for len(appConfig.GitServers) < 5 {
		appConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
	}

	if appConfig.MimeTypes == nil {
		appConfig.MimeTypes = map[string]string{
			".css":   "text/css",
			".js":    "application/javascript",
			".json":  "application/json",
			".woff2": "font/woff2",
		}
		data, _ := json.MarshalIndent(appConfig, "", "  ")
		os.WriteFile(configPath, data, 0644)
	}

}
