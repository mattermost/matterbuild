package scpwrapper

import (
	"github.com/eugenmayer/go-scp"
	"golang.org/x/crypto/ssh"
)

// scp a file from a remote server using ssh / scp
func CopyFromRemote(source string, dest string, client *ssh.Client) error {
	scpClient := scp.NewSCP(client)
	return scpClient.ReceiveFile(source, dest)
}
