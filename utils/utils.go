// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

func MilisecsToMinutes(value int64) string {
	str := fmt.Sprintf("%v", time.Duration(value)*time.Millisecond)
	return str
}

func Even(number int) bool {
	return number%2 == 0
}

func Odd(number int) bool {
	return !Even(number)
}

// Pipe stdout of each command into stdin of next.
func AssemblePipes(cmds []*exec.Cmd, stdin io.Reader, stdout io.Writer) []*exec.Cmd {
	var errBuffer bytes.Buffer
	cmds[0].Stdin = stdin
	cmds[0].Stderr = bufio.NewReadWriter(bufio.NewReader(&errBuffer), bufio.NewWriter(&errBuffer))
	// assemble pipes
	for i, c := range cmds {
		var errBuffer bytes.Buffer

		if i < len(cmds)-1 {
			cmds[i+1].Stdin, _ = c.StdoutPipe()
			cmds[i+1].Stderr = bufio.NewReadWriter(bufio.NewReader(&errBuffer), bufio.NewWriter(&errBuffer))
		} else {
			c.Stdout = stdout
			c.Stderr = bufio.NewReadWriter(bufio.NewReader(&errBuffer), bufio.NewWriter(&errBuffer))
		}
	}
	return cmds
}

// Run series of piped commands.
func RunCmds(cmds []*exec.Cmd) error {
	// start processes in descending order
	for i := len(cmds) - 1; i > 0; i-- {
		if err := cmds[i].Start(); err != nil {
			return err
		}
	}
	// run the first process
	if err := cmds[0].Run(); err != nil {
		return err
	}
	// wait on processes in ascending order
	for i := 1; i < len(cmds); i++ {
		if err := cmds[i].Wait(); err != nil {
			// Read error details
			readWriter, ok := cmds[i].Stderr.(*bufio.ReadWriter)
			if !ok {
				return errors.New("cmd stderr is not *bufio.ReadWriter")
			}

			errData, otherErr := ioutil.ReadAll(readWriter)
			if otherErr != nil {
				return errors.Wrapf(otherErr, "failed to read stdErr from cmd")
			}

			return errors.Wrapf(err, "cmd stdErr=%s", errData)
		}
	}
	return nil
}
