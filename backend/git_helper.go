package backend

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func GetInsecureSSHAuth(sshUser, privateKeyPath, password string) (*ssh.PublicKeys, error) {
	_, err := os.Stat(privateKeyPath)
	if err != nil {
		return nil, err
	}
	publicKeys, err := ssh.NewPublicKeysFromFile(sshUser, privateKeyPath, password)
	if err != nil {
		return nil, err
	}
	
	signer := publicKeys.Signer
	pubKeyBytes := gossh.MarshalAuthorizedKey(signer.PublicKey())
	pubKeyStr := strings.TrimSpace(string(pubKeyBytes))
	
	log.Printf("\n[CRITICAL] To fix 'unable to authenticate', add THIS EXACT KEY to your gitolite-admin repo:")
	log.Printf("[CRITICAL] %s", pubKeyStr)
	log.Printf("[CRITICAL] Your desktop CLI likely succeeded by silently falling back to ~/.ssh/id_rsa!\n")

	publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
	return publicKeys, nil
}

func GetForcePullAndReset() bool {
	configPath := "data/config.json"
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(configData, &cfg); err != nil {
		return false
	}
	
	force := false
	// FIX: Safely parse both actual booleans and stringified booleans from the JS UI
	if valBool, ok := cfg["force_pull_one_time"].(bool); ok {
		force = valBool
	} else if valStr, ok := cfg["force_pull_one_time"].(string); ok {
		force = (valStr == "true" || valStr == "on")
	}
	
	if force {
		log.Printf("[SYNC] ForcePullOneTime detected! Executing destructive Force Pull and resetting flag to false.")
		cfg["force_pull_one_time"] = false
		if newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			os.WriteFile(configPath, newData, 0644)
		}
	}
	return force
}

func GetConfigAuthor() string {
	author := "OMN-Go App"
	configPath := "data/config.json"
	if configData, err := os.ReadFile(configPath); err == nil {
		var cfg map[string]interface{}
		if json.Unmarshal(configData, &cfg) == nil {
			if val, ok := cfg["author"].(string); ok && val != "" {
				author = val
			} else if val, ok := cfg["Author"].(string); ok && val != "" {
				author = val
			}
		}
	}
	return author
}
