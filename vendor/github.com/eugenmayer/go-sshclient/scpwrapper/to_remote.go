package scpwrapper

import (
	"fmt"
	"github.com/kballard/go-shellquote"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path"
)
// scp a file to a remote server using ssh / scp
func CopyToRemote(source string, dest string, session *ssh.Session) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	defer session.Close()
	w, err := session.StdinPipe()

	if err != nil {
		return err
	}

	destination := path.Dir(dest)
	cmd := shellquote.Join("scp", "-t", destination)
	if err := session.Start(cmd); err != nil {
		w.Close()
		return err
	}

	errors := make(chan error)

	go func() {
		errors <- session.Wait()
	}()

	fmt.Fprintf(w, "C%#o %d %s\n", stat.Mode().Perm(), stat.Size(), path.Base(dest))
	io.Copy(w, f)
	fmt.Fprint(w, "\x00")
	w.Close()

	return <-errors
}