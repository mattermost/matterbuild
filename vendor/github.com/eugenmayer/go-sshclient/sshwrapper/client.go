package sshwrapper

import (
	"errors"
	"fmt"
	"github.com/eugenmayer/go-sshclient/scpwrapper"
	"golang.org/x/crypto/ssh"
	"net"
	"time"
)

// connect the ssh client and create a session, ready to go with commands ro scp
func (sshApi *SshApi) ConnectAndSession() (err error) {
	if client, err := sshApi.Connect(); err != nil {
		if client != nil {
			sshApi.Client.Close()
		}
		return err
	} else {
		sshApi.Client = client
	}

	err = sshApi.SessionDefault()
	if err != nil {
		if sshApi.Client != nil {
			sshApi.Client.Close()
		}
		return err
	}
	return nil
}

// creates a default session with usual parameters
func (sshApi *SshApi) SessionDefault() (err error) {
	if session, err := sshApi.Client.NewSession(); err != nil {
		return err
	} else {
		sshApi.Session = session
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := sshApi.Session.RequestPty("xterm", 80, 40, modes); err != nil {
		sshApi.Session.Close()
		return err
	}

	sshApi.Session.Stdout = &sshApi.StdOut
	sshApi.Session.Stderr = &sshApi.StdErr
	return nil
}

// connect the ssh client - use ConnectAndSession if you have no reason to create
// the session manually why ever
// we do support proper timeouts here, thats why it looks a little more complicated then the usual ssh connect
func (sshApi *SshApi) Connect() (*ssh.Client, error) {
	var addr = fmt.Sprintf("%s:%d", sshApi.Host, sshApi.Port)
	conn, err := net.DialTimeout("tcp", addr, sshApi.SshConfig.Timeout)
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(sshApi.SshConfig.Timeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, sshApi.SshConfig)
	if err != nil {
		return nil, err
	}

	err = conn.SetReadDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), err
}

// get the stdout from your last command
func (sshApi *SshApi) GetStdOut() string {
	return sshApi.StdOut.String()
}

// get the stderr from your last command
func (sshApi *SshApi) GetStdErr() string {
	return sshApi.StdErr.String()
}

// run a ssh command. Auto-creates session if you did yet not connect
// just wrapping ssh.Session.Run with connect / run and then disconnect
func (sshApi *SshApi) Run(cmd string) (stdout string, stderr string, err error) {
	if sshApi.Session == nil {
		if err = sshApi.ConnectAndSession(); err != nil {
			sshApi.Close()
			return "","", err
		}

		// this can actually still happen. TODO: document why
		if sshApi.Session == nil {
			sshApi.Close()
			return "","", errors.New("could not start ssh session")
		}
	}

	sshApi.StdOut.Reset()
	sshApi.StdErr.Reset()
	err = sshApi.Session.Run(cmd)

	sshApi.Close()
	return  sshApi.GetStdOut(),sshApi.GetStdErr(), err
}

// scp a local file to a remote host
func (sshApi *SshApi) CopyToRemote(source string, dest string) (err error) {
	sshApi.Client, err = sshApi.Connect()
	if err != nil {
		sshApi.Close()
		return err
	}
	sshApi.Session, err = sshApi.Client.NewSession()
	if err != nil {
		sshApi.Close()
		return err
	}
	err = scpwrapper.CopyToRemote(source, dest, sshApi.Session)
	defer sshApi.Close()
	return err
}

// scp a file from a remote host
func (sshApi *SshApi) CopyFromRemote(source string, dest string) (err error) {
	sshApi.Client, err = sshApi.Connect()
	if err != nil {
		sshApi.Close()
		return err
	}
	//sshApi.Session, err = sshApi.Client.NewSession()
	//if err != nil {
	//	sshApi.Close()
	//	return err
	//}

	err = scpwrapper.CopyFromRemote(source, dest, sshApi.Client)
	sshApi.Close()
	return err
}

func (sshApi *SshApi) Close() (err error) {
	// we need to always reset our internal pointer to session since the ssh crypto library is not
	// designed to reused it or "close" it at all .Close does close the connection but does not
	// cleanup the session, more precisely .started is never set to false.
	sshApi.Session = nil

	if sshApi.Client != nil {
		err = sshApi.Client.Close()
	}
	return err
}