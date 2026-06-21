package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ServerPort    int               `json:"server_port"`
	AdminPassword string            `json:"admin_password"`
	GuestPassword string            `json:"guest_password"`
	Author        string            `json:"author"`
	UseInternalEd bool              `json:"use_internal_editor"`
	DesktopExtCmd string            `json:"desktop_ext_cmd"`
	MimeTypes     map[string]string `json:"mime_types"`
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
		if appConfig.ServerPort == 0 {
			appConfig.ServerPort = 8080
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
}
