package backend

import (
	"os"
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
	return publicKeys, nil
}
