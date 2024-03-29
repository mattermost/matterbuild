// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
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
	S3ReleaseBucket string

	AllowedTokens []string
	AllowedUsers  []string
	ReleaseUsers  []string

	PluginSigningSSHPublicCertPath string // Used for local development
	PluginSigningSSHKeyPath        string
	PluginSigningSSHUser           string
	PluginSigningSSHHost           string
	PluginSigningSSHHostPublicKey  string

	PluginSigningAWSAccessKey      string
	PluginSigningAWSSecretKey      string
	PluginSigningAWSRegion         string
	PluginSigningAWSS3PluginBucket string

	CIServerJenkinsUserName string
	CIServerJenkinsToken    string
	CIServerJenkinsURL      string
	CIServerJobs            []string

	ReleaseJob                string
	ReleaseJobLegacy          string
	RCTestingJob              string
	TranslationServerJob      string
	CheckTranslationServerJob string
	GithubAccessToken         string
	GithubUsername            string
	GithubOrg                 string
	Repositories              []*Repository

	KubeDeployJob string

	PipelineTriggers map[string]*PipelineTrigger
}

type Repository struct {
	Owner string
	Name  string
}

type PipelineTrigger struct {
	Description string
	URL         string
	Token       string
	Reference   string
	Variables   map[string]string
	Users       map[string]string
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
	LogInfo("Loading " + fileName)

	file, err := os.Open(fileName)
	if err != nil {
		LogError("Error opening config file=" + fileName + ", err=" + err.Error())
	}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(Cfg)
	if err != nil {
		LogError("Error decoding config file=" + fileName + ", err=" + err.Error())
	}
}
