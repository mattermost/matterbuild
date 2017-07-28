// Copyright (c) 2017 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type MatterbuildConfig struct {
	ListenAddress   string
	JenkinsURL      string
	JenkinsUsername string
	JenkinsPassword string

	AllowedTokens []string
	AllowedUsers  []string
	ReleaseUsers  []string

	CIServerJobs []string

	ReleaseJob   string
	PreChecksJob string

	PreReleaseJob string

	KubeDeployJob string
}

var Cfg *MatterbuildConfig = &MatterbuildConfig{}

func FindConfigFile(fileName string) string {
	if _, err := os.Stat("./config/" + fileName); err == nil {
		fileName, _ = filepath.Abs("./config/" + fileName)
	} else if _, err := os.Stat("../config/" + fileName); err == nil {
		fileName, _ = filepath.Abs("../config/" + fileName)
	} else if _, err := os.Stat(fileName); err == nil {
		fileName, _ = filepath.Abs(fileName)
	}

	return fileName
}

func LoadConfig(fileName string) {
	fileName = FindConfigFile(fileName)
	Info("Loading " + fileName)

	file, err := os.Open(fileName)
	if err != nil {
		Error("Error opening config file=" + fileName + ", err=" + err.Error())
	}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(Cfg)
	if err != nil {
		Error("Error decoding config file=" + fileName + ", err=" + err.Error())
	}
}
