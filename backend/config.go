package backend

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// maxGitServers is the fixed number of git-server config slots the UI
// exposes. It used to be a literal "5" repeated in four different places
// (this file, handleConfig's POST handler, and getConfigPageBody) that all
// had to be kept in sync by hand; centralizing it here means changing the
// slot count is a one-line change.
const maxGitServers = 5

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
	a.ConfigMutex.Lock()
	defer a.ConfigMutex.Unlock()

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
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			log.Printf("loadConfig: failed to marshal default config: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("loadConfig: failed to write default config.json: %v", err)
		}
	} else {
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			// Can't read an existing config.json - leave a.Config at its
			// zero value and say so loudly, rather than silently running
			// with an empty/broken config that looks intentional.
			log.Printf("loadConfig: failed to read %s: %v", configPath, readErr)
		} else if err := json.Unmarshal(data, &a.Config); err != nil {
			// A corrupt config.json used to be swallowed here, leaving
			// a.Config partially or fully zeroed with no indication why.
			// Log it clearly so a bad file is obvious instead of looking
			// like passwords/settings mysteriously reset themselves.
			log.Printf("loadConfig: failed to parse %s (using defaults for any unparsed fields): %v", configPath, err)
		}
		// [OMN-Go 1.5.21] Absolute Array Lock: Prevents the JSON 'null' wipe bug forever
		for len(a.Config.GitServers) < maxGitServers {
			a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
		}

	}
	if a.Config.ServerPort == 0 {
		a.Config.ServerPort = 8080
	}
	// [OMN-Go 1.5.16] Enforce maxGitServers empty slots natively
	for len(a.Config.GitServers) < maxGitServers {
		a.Config.GitServers = append(a.Config.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(a.Config.GitServers)+1)})
	}

	if a.Config.MimeTypes == nil {
		a.Config.MimeTypes = map[string]string{
			".css":   "text/css",
			".js":    "application/javascript",
			".json":  "application/json",
			".woff2": "font/woff2",
		}
		data, err := json.MarshalIndent(a.Config, "", "  ")
		if err != nil {
			log.Printf("loadConfig: failed to marshal config after mime-type fixup: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("loadConfig: failed to write config.json after mime-type fixup: %v", err)
		}
	}

}

