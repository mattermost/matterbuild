// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/bndr/gojenkins"
	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"github.com/spf13/cobra"

	"github.com/mattermost/matterbuild/utils"
)

const (
	IN_CHANNEL = "in_channel"
	EPHEMERAL  = "ephemeral"
)

type MMSlashCommand struct {
	ChannelId   string `schema:"channel_id"`
	ChannelName string `schema:"channel_name"`
	Command     string `schema:"command"`
	TeamName    string `schema:"team_domain"`
	TeamId      string `schema:"team_id"`
	Text        string `schema:"text"`
	Token       string `schema:"token"`
	UserId      string `schema:"user_id"`
	Username    string `schema:"user_name"`
}

type AppError struct {
	ErrorDescription string
	Parent           error
}

func (err *AppError) Error() string {
	if err == nil {
		return "No Error (nil)"
	}

	if err.Parent != nil {
		return err.ErrorDescription + " |:| " + err.Parent.Error()
	}

	return err.ErrorDescription
}

func NewError(description string, parent error) *AppError {
	return &AppError{
		ErrorDescription: description,
		Parent:           parent,
	}
}

func Error(err string) {
	fmt.Println("[ERROR] " + err)
}

func Info(info string) {
	fmt.Println("[INFO] " + info)
}

func WriteErrorResponse(w http.ResponseWriter, err *AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(GenerateStandardSlashResponse(err.Error(), EPHEMERAL)))
}

func WriteResponse(w http.ResponseWriter, resp string, style string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(GenerateStandardSlashResponse(resp, style)))
}

func WriteEnrichedResponse(w http.ResponseWriter, title, resp, color, style string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(GenerateEnrichedSlashResponse(title, resp, color, style)))
}

func ParseSlashCommand(r *http.Request) (*MMSlashCommand, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, err
	}
	inCommand := &MMSlashCommand{}
	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)

	err = decoder.Decode(inCommand, r.Form)
	if err != nil {
		return nil, err
	}

	return inCommand, nil
}

func Start() {
	LoadConfig("config.json")
	LogInfo("Starting Matterbuild")

	router := httprouter.New()
	router.GET("/", indexHandler)
	router.POST("/slash_command", slashCommandHandler)

	LogInfo("Running Matterbuild on port " + Cfg.ListenAddress)
	http.ListenAndServe(Cfg.ListenAddress, router)

}

func indexHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Write([]byte("This is the matterbuild server."))
}

func checkSlashPermissions(command *MMSlashCommand) *AppError {
	hasPremissions := false
	for _, allowedToken := range Cfg.AllowedTokens {
		if allowedToken == command.Token {
			hasPremissions = true
			break
		}
	}

	if !hasPremissions {
		return NewError("Token for slash command is incorrect", nil)
	}

	hasPremissions = false
	for _, allowedUser := range Cfg.AllowedUsers {
		if allowedUser == command.UserId {
			hasPremissions = true
			break
		}
	}

	if !hasPremissions {
		return NewError("You don't have permissions to use this command.", nil)
	}

	if command.Command == "cut" {
		hasPremissions = false
		for _, allowedUser := range Cfg.ReleaseUsers {
			if allowedUser == command.UserId {
				hasPremissions = true
				break
			}
		}

		if !hasPremissions {
			return NewError("You don't have permissions to use this command.", nil)
		}
	}

	return nil
}

func slashCommandHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	command, err := ParseSlashCommand(r)
	if err != nil {
		WriteErrorResponse(w, NewError("Unable to parse incoming slash command info", err))
		return
	}

	if err := checkSlashPermissions(command); err != nil {
		WriteErrorResponse(w, err)
		return
	}

	// Output Buffer
	outBuf := &bytes.Buffer{}

	var rootCmd = &cobra.Command{
		Use:   "matterbuild",
		Short: "Control of the build system though MM slash commands!",
	}

	var cutCmd = &cobra.Command{
		Use:   "cut [release]",
		Short: "Cut a release of Mattermost",
		Long:  "Cut a release of Mattermost. Version should be specified in the format 0.0.0-rc0 or 0.0.0 for final releases.",
		RunE: func(cmd *cobra.Command, args []string) error {
			backport, _ := cmd.Flags().GetBool("backport")
			dryrun, _ := cmd.Flags().GetBool("dryrun")
			legacy, _ := cmd.Flags().GetBool("legacy")
			server, _ := cmd.Flags().GetString("server")
			webapp, _ := cmd.Flags().GetString("webapp")
			return cutReleaseCommandF(args, w, command, backport, dryrun, legacy, server, webapp)
		},
	}
	cutCmd.Flags().Bool("backport", false, "Set this flag for releases that are not on the current major release branch.")
	cutCmd.Flags().Bool("dryrun", false, "Set this flag for testing the release build without pushing tags or artifacts.")
	cutCmd.Flags().Bool("legacy", false, "Set this flag to build release older then release number 5.7.x.")
	cutCmd.Flags().String("server", "", "Set this flag to define the Docker image used to build the server. Optional the job will use the hardcoded one if not defined")
	cutCmd.Flags().String("webapp", "", "Set this flag to define the Docker image used to build the webapp. Optional the job will use the hardcoded one if not defined")

	var configDumpCmd = &cobra.Command{
		Use:   "seeconf",
		Short: "Dump the configuration of a build job.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configDumpCommandF(args, w, command)
		},
	}

	var setCIBranchCmd = &cobra.Command{
		Use:   "setci",
		Short: "Set the branch target for the CI servers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setCIBranchCmdF(args, w, command)
		},
	}

	var runJobCmd = &cobra.Command{
		Use:   "runjob",
		Short: "Run a job on Jenkins.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobCmdF(args, w, command)
		},
	}

	var checkCutReleaseStatusCmd = &cobra.Command{
		Use:   "cutstatus",
		Short: "Check the status of the Cut Release Job",
		RunE: func(cmd *cobra.Command, args []string) error {
			legacy, _ := cmd.Flags().GetBool("legacy")
			return checkCutReleaseStatusF(args, w, command, legacy)
		},
	}

	var lockTranslationServerCmd = &cobra.Command{
		Use:   "lockpootle",
		Short: "Lock the Translation server for a particular release Branch",
		Long:  "Lock the Translation server for a particular release Branch or to master.",
		RunE: func(cmd *cobra.Command, args []string) error {
			plt, _ := cmd.Flags().GetString("plt")
			web, _ := cmd.Flags().GetString("web")
			mobile, _ := cmd.Flags().GetString("mobile")
			return lockTranslationServerCommandF(args, w, command, plt, web, mobile)
		},
	}
	lockTranslationServerCmd.Flags().String("plt", "", "Set this flag to set the translation server to lock the server repo")
	lockTranslationServerCmd.Flags().String("web", "", "Set this flag to set the translation server to lock the webapp repo")
	lockTranslationServerCmd.Flags().String("mobile", "", "Set this flag to set the translation server to lock the mobile repo")

	var checkBranchTranslationCmd = &cobra.Command{
		Use:   "getpootle",
		Short: "Check the branches set in the Translation Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkBranchTranslationCmdF(args, w, command)
		},
	}

	rootCmd.SetArgs(strings.Fields(strings.TrimSpace(command.Text)))
	rootCmd.SetOutput(outBuf)

	rootCmd.AddCommand(cutCmd, configDumpCmd, setCIBranchCmd, runJobCmd, checkCutReleaseStatusCmd, lockTranslationServerCmd, checkBranchTranslationCmd)

	err = rootCmd.Execute()

	if err != nil || len(outBuf.String()) > 0 {
		WriteEnrichedResponse(w, "Information", outBuf.String(), "#0060aa", EPHEMERAL)
	}
	return
}

var finalVersionRxp = regexp.MustCompile("^[0-9]+.[0-9]+.[0-9]+$")
var rcRxp = regexp.MustCompile("^[0-9]+.[0-9]+.[0-9]+-rc[0-9]+$")

func cutReleaseCommandF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand, backport bool,
	dryrun bool, legacy bool, server string, webapp string) error {
	if len(args) < 1 {
		return NewError("You need to specifiy a release version.", nil)
	}

	versionString := args[0]

	// Check the version string given and split into release part (0.0.0) and rc part (rc0)
	// Also determine if this is RC1 of a .0 build in which case we need to branch
	var releasePart string
	var rcPart string
	var isFirstMinorRelease bool

	if rcRxp.MatchString(versionString) {
		split := strings.Split(versionString, "-")
		if len(split) != 2 {
			WriteErrorResponse(w, NewError("Bad version argument. Can't split on -. Typo? If not the regex might be broken. If so be more careful!!", nil))
			return nil
		}
		releasePart = split[0]
		rcPart = split[1]
		isFirstMinorRelease = (rcPart == "rc1" && strings.HasSuffix(releasePart, ".0"))
	} else if finalVersionRxp.MatchString(versionString) {
		releasePart = versionString
		rcPart = ""
		isFirstMinorRelease = false
	} else {
		WriteErrorResponse(w, NewError("Bad version argument. Typo? If not the regex might be broken. If so be more careful!!", nil))
		return nil
	}

	// Check that the release dev hasn't forgotten to get --backport
	if !backport {
		splitRelease := strings.Split(releasePart, ".")
		if len(splitRelease) != 3 {
			WriteErrorResponse(w, NewError("Bad version argument.", nil))
			return nil
		}
		intVer, err := strconv.Atoi(splitRelease[1])
		if err != nil {
			WriteErrorResponse(w, NewError("Bad version argument.", nil))
			return nil
		}
		splitRelease[1] = strconv.Itoa(intVer + 1)
		splitRelease[2] = "0"

		oneReleaseUp := strings.Join(splitRelease, ".")

		s3URL := "http://releases.mattermost.com/" + oneReleaseUp + "-rc1/mattermost-" + oneReleaseUp + "-rc1-linux-amd64.tar.gz"
		if resp, err := http.Get(s3URL); err == nil && resp.StatusCode == http.StatusOK {
			WriteErrorResponse(w, NewError("Are you sure this isn't a backport release? I see a future release on s3. ("+oneReleaseUp+")"+http.StatusText(resp.StatusCode), nil))
			return nil
		}
	}

	err := CutRelease(releasePart, rcPart, isFirstMinorRelease, backport, dryrun, legacy, server, webapp)
	if err != nil {
		WriteErrorResponse(w, err)
	} else {
		msg := fmt.Sprintf("Release **%v** is on the way.", args[0])
		WriteEnrichedResponse(w, "Cut Release", msg, "#0060aa", IN_CHANNEL)
	}
	return nil
}

func configDumpCommandF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to supply an argument", nil)
	}

	config, err := GetJobConfig(args[0], Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
	if err != nil {
		return err
	}

	LogInfo("Config Dump sent... dump=" + config)

	WriteResponse(w, config, IN_CHANNEL)
	return nil
}

func setCIBranchCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a branch", nil)
	}

	if err := SetCIServerBranch(args[0]); err != nil {
		LogError("Error when setting the branch. err= " + err.Error())
		return err
	}

	LogInfo("CI servers now pointed at " + args[0])
	msg := fmt.Sprintf("CI servers now pointed at **%v**", args[0])
	WriteEnrichedResponse(w, "CI Servers", msg, "#0060aa", IN_CHANNEL)
	return nil
}

func runJobCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a job", nil)
	}

	if err := RunJob(args[0]); err != nil {
		return err
	}

	msg := fmt.Sprintf("Ran job **%v**", args[0])
	WriteEnrichedResponse(w, "Jenkins Job", msg, "#0060aa", IN_CHANNEL)
	return nil
}

func checkCutReleaseStatusF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand, legacy bool) error {
	var jobName string
	if legacy {
		jobName = Cfg.ReleaseJobLegacy
	} else {
		jobName = Cfg.ReleaseJob
	}
	LogInfo("Running Check Cut Release Status")
	status, err := GetLatestResult(jobName)
	if err != nil {
		LogError("[checkCutReleaseStatusF] Unable to get the Job: " + jobName + " err=" + err.Error())
		return err
	}

	msg := fmt.Sprintf("Status of *%v*: **%v** Duration: **%v**", jobName, status.Status, utils.MilisecsToMinutes(status.Duration))

	WriteEnrichedResponse(w, "Status of Jenkins Job", msg, status.Color, IN_CHANNEL)
	return nil
}

func lockTranslationServerCommandF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand, plt, web, mobile string) error {

	if plt == "" && web == "" && mobile == "" {
		msg := "You need to set at least one branch to lock. Please check the help."
		WriteEnrichedResponse(w, "Translation Server Update", msg, "#ee2116", IN_CHANNEL)
		return nil
	}

	msg := "Jenkins Job is running but we set the translation Server to those Branches:\n"
	if plt != "" {
		msg += fmt.Sprintf("* Server Branch: **%v**\n", plt)
	}
	if web != "" {
		msg += fmt.Sprintf("* Webapp Branch: **%v**\n", web)
	}
	if mobile != "" {
		msg += fmt.Sprintf("* Mobile Branch: **%v**\n", mobile)
	}

	WriteEnrichedResponse(w, "Translation Server Update", msg, "#0060aa", IN_CHANNEL)

	result, err := RunJobWaitForResult(
		Cfg.TranslationServerJob,
		map[string]string{
			"PLT_BRANCH": plt,
			"WEB_BRANCH": web,
			"RN_BRANCH":  mobile,
		})
	if err != nil || result != gojenkins.STATUS_SUCCESS {
		LogError("Translation job failed. err= " + err.Error() + " Jenkins result= " + result)
	}

	return nil
}

func checkBranchTranslationCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	LogInfo("Will run the job to get the information about the branches in the translation server")
	result, err := RunJobWaitForResult(Cfg.CheckTranslationServerJob, map[string]string{})
	if err != nil || result != gojenkins.STATUS_SUCCESS {
		LogError("Translation job failed. err= " + err.Error() + " Jenkins result= " + result)
		msg := fmt.Sprintf("Translation Job Fail. Please Check the Jenkins Logs. Jenkins Status: %v", result)
		WriteEnrichedResponse(w, "Translation Server Update", msg, "#ee2116", IN_CHANNEL)
		return nil
	}

	LogInfo("Will get the artificat from jenkins")
	artifacts, err := GetJenkinsArtifacts(Cfg.CheckTranslationServerJob)
	if err != nil {
		return err
	}

	if len(artifacts) == 0 {
		LogError("Artifact is empty")
		return fmt.Errorf("Artifact is empty")
	}

	_, errSave := artifacts[0].SaveToDir("/tmp")
	if errSave != nil {
		LogError("Error saving the artifact to /tmp")
		return errSave
	}

	LogInfo("Artifact - " + artifacts[0].FileName)

	file := fmt.Sprintf("/tmp/%v", artifacts[0].FileName)
	dat, errFile := ioutil.ReadFile(file)
	if errFile != nil {
		LogError("Error reading the file. err= " + errFile.Error())
	}

	LogInfo("Results %s", string(dat))
	tmpMsg := string(dat)
	tmpMsg = strings.Replace(tmpMsg, "PLT_BRANCH=", "Server Branch:", -1)
	tmpMsg = strings.Replace(tmpMsg, "WEB_BRANCH=", "Webapp Branch:", -1)
	tmpMsg = strings.Replace(tmpMsg, "RN_BRANCH=", "Mobile Branch:", -1)
	tmpMsg = strings.Replace(tmpMsg, "\"", " **", -1)
	splittedMsg := strings.Split(tmpMsg, "\n")
	msg := "Translation Server have lock to those Branches:\n"
	for _, txt := range splittedMsg {
		msg += fmt.Sprintf("%v\n", txt)
	}

	WriteEnrichedResponse(w, "Translation Server Update", msg, "#0060aa", IN_CHANNEL)

	return nil
}
