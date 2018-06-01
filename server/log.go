// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"fmt"
	"log"
	"os"

	l4g "github.com/alecthomas/log4go"
)

func LogInfo(msg string, args ...interface{}) {
	l4g.Info(msg, args...)
	Log("INFO", msg, args...)
}

func LogError(msg string, args ...interface{}) {
	l4g.Error(msg, args...)
	Log("ERROR", msg, args...)
}

func LogCritical(msg string, args ...interface{}) {
	l4g.Critical(msg, args...)
	Log("CRIT", msg, args...)
	panic(fmt.Sprintf(msg, args...))
}

func Log(level string, msg string, args ...interface{}) {
	log.Printf("%v %v\n", level, fmt.Sprintf(msg, args...))
	f, err := os.OpenFile("matterbuild.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to write to file")
		return
	}
	defer f.Close()

	log.SetOutput(f)
	log.Printf("%v %v\n", level, fmt.Sprintf(msg, args...))
}
