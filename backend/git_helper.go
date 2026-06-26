package backend

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// GetInsecureSSHAuth bypasses strict host key checking which blocks Android from connecting to gitolite
func GetInsecureSSHAuth(sshUser, privateKeyPath, password string) (*ssh.PublicKeys, error) {
	_, err := os.Stat(privateKeyPath)
	if err != nil {
		return nil, err
	}
	publicKeys, err := ssh.NewPublicKeysFromFile(sshUser, privateKeyPath, password)
	if err != nil {
		return nil, err
	}
	
	// EXPLICIT PUBKEY EXTRACTION
	signer := publicKeys.Signer
	pubKeyBytes := gossh.MarshalAuthorizedKey(signer.PublicKey())
	pubKeyStr := strings.TrimSpace(string(pubKeyBytes))
	
	log.Printf("\n[CRITICAL] To fix 'unable to authenticate', add THIS EXACT KEY to your gitolite-admin repo:")
	log.Printf("[CRITICAL] %s", pubKeyStr)
	log.Printf("[CRITICAL] Your desktop CLI likely succeeded by silently falling back to ~/.ssh/id_rsa!\n")

	// CRITICAL FIX: Ignore host key verification for gitolite3 servers
	publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
	return publicKeys, nil
}

// GetForcePullAndReset reads config.json, checks the one-time flag, and auto-resets it to false.
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
	
	force, ok := cfg["force_pull_one_time"].(bool)
	if force {
		log.Printf("[SYNC] ForcePullOneTime detected! Executing destructive Force Pull and resetting flag to false.")
		cfg["force_pull_one_time"] = false
		if newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			os.WriteFile(configPath, newData, 0644)
		}
	}
	return force
}

// GetConfigAuthor dynamically extracts the author from the config JSON.
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
