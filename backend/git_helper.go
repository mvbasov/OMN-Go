package backend

import (
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
	
	// EXPLICIT PUBKEY EXTRACTION: Output the exact string needed for gitolite-admin
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
