package sshwrapper

import (
	"bytes"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"net"
	"os"
	"time"
)

type SshApi struct {
	SshConfig *ssh.ClientConfig
	Client    *ssh.Client
	Session   *ssh.Session
	User      string
	Key       string
	Password  string
	Host      string
	Port      int
	Timeout   time.Duration
	StdOut    bytes.Buffer
	StdErr    bytes.Buffer
}

func NewSshApi(host string, port int, user string, key string) (sshApi *SshApi) {
	sshApi = &SshApi{
		User:    user,
		Key:     key,
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	}
	return sshApi
}

func DefaultSshApiSetup(host string, port int, user string, key string) (sshApi *SshApi, err error) {
	sshApi = NewSshApi(host, port, user, key)

	if key == "" {
		err = sshApi.defaultSshAgentSetup()
	} else {
		err = sshApi.defaultSshPrivkeySetup()
	}
	return sshApi, err
}

func (sshApi *SshApi) DefaultSshPasswordSetup() error {
	sshApi.SshConfig = &ssh.ClientConfig{
		User:            sshApi.User,
		Auth:            []ssh.AuthMethod{ssh.Password(sshApi.Password)},
		Timeout:         sshApi.Timeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return nil
}

func (sshApi *SshApi) defaultSshAgentSetup() error {
	sshAgent, err := SSHAgent()
	if err != nil {
		return err
	}

	sshApi.SshConfig = &ssh.ClientConfig{
		User:            sshApi.User,
		Auth:            []ssh.AuthMethod{sshAgent},
		Timeout:         sshApi.Timeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return nil
}

func (sshApi *SshApi) defaultSshPrivkeySetup() error {
	buffer, err := ioutil.ReadFile(sshApi.Key)
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return err
	}

	// Load the certificate
	cert, err := ioutil.ReadFile("/Users/alifarooq/workspace/go/src/github.com/mattermost/platform/signed-cert.pub")
	if err != nil {
		return err
	}

	pk, _, _, _, err := ssh.ParseAuthorizedKey(cert)
	if err != nil {
		return err
	}

	certSigner, err := ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
	if err != nil {
		return err
	}

	timeout := sshApi.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	sshApi.SshConfig = &ssh.ClientConfig{
		User: sshApi.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(certSigner),
		},
		Timeout:         sshApi.Timeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return nil
}

func SSHAgent() (ssh.AuthMethod, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if sshAgent, err := net.Dial("unix", socket); err != nil {
		return nil, err
	} else {
		return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers), nil
	}
}

func LoadPrivateKeyFile(file string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}
