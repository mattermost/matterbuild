// Copyright (c) 2017 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"github.com/spf13/cobra"
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

type MMSlashResponse struct {
	ResponseType string `json:"response_type"`
	Text         string `json:"text"`
	GotoLocation string `json:"goto_location"`
	Username     string `json:"username"`
	IconURL      string `json:"icon_url"`
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

func GenerateStandardSlashResponse(text string) string {
	response := MMSlashResponse{
		ResponseType: "in_channel",
		Text:         text,
		GotoLocation: "",
		Username:     "Matterbuild",
		IconURL:      "https://www.mattermost.org/wp-content/uploads/2016/04/icon.png",
	}

	b, err := json.Marshal(response)
	if err != nil {
		Error("Unable to marshal response")
		return ""
	}

	return string(b)
}

func WriteErrorResponse(w http.ResponseWriter, err *AppError) {
	w.Write([]byte(GenerateStandardSlashResponse(err.Error())))
}

func WriteResponse(w http.ResponseWriter, resp string) {
	w.Write([]byte(GenerateStandardSlashResponse(resp)))
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

	router := httprouter.New()
	router.GET("/", indexHandler)
	router.POST("/slash_command", slashCommandHandler)

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
			return curReleaseCommandF(args, w, command, backport)
		},
	}
	cutCmd.Flags().Bool("backport", false, "Set this flag for releases that are not on the current major release branch.")

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

	var setPreReleaseCmd = &cobra.Command{
		Use:   "setprerelease",
		Short: "Set the target for pre-release.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setPreReleaseCmdF(args, w, command)
		},
	}

	var loadtestKubeCmd = &cobra.Command{
		Use:   "loadtest [buildtag]",
		Short: "Create a kubernetes cluster to loadtest a branch or pr.",
		Long:  "Creates a kubernetes cluster to loadtest a branch or pr. buildtag must be a branch name or pr-0000 where 0000 is the PR number in github. Note that the branch or PR must have built before this command can be run.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return loadtestKubeF(args, w, command)
		},
	}

	rootCmd.SetArgs(strings.Fields(strings.TrimSpace(command.Text)))
	rootCmd.SetOutput(outBuf)

	rootCmd.AddCommand(cutCmd, configDumpCmd, setCIBranchCmd, runJobCmd, setPreReleaseCmd, loadtestKubeCmd)

	rootCmd.Execute()

	WriteResponse(w, outBuf.String())
}

var finalVersionRxp = regexp.MustCompile("^[0-9]+.[0-9]+.[0-9]+$")
var rcRxp = regexp.MustCompile("^[0-9]+.[0-9]+.[0-9]+-rc[0-9]+$")

func curReleaseCommandF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand, backport bool) error {
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

	if err := CutRelease(releasePart, rcPart, isFirstMinorRelease, backport); err != nil {
		WriteErrorResponse(w, err)
	} else {
		WriteResponse(w, "Release "+args[0]+" is on the way.")
	}
	return nil
}

func configDumpCommandF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to supply an argment", nil)
	}

	config, err := GetJobConfig(args[0])
	if err != nil {
		return err
	}

	WriteResponse(w, config)
	return nil
}

func setCIBranchCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a branch", nil)
	}

	if err := SetCIServerBranch(args[0]); err != nil {
		return err
	}

	WriteResponse(w, "CI servers now pointed at "+args[0])
	return nil
}

func runJobCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a job", nil)
	}

	if err := RunJob(args[0]); err != nil {
		return err
	}

	WriteResponse(w, "Ran job "+args[0])
	return nil
}

func setPreReleaseCmdF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a target", nil)
	}

	if err := SetPreReleaseTarget(args[0]); err != nil {
		return err
	}

	WriteResponse(w, "Set pre-release to "+args[0])
	return nil
}

func loadtestKubeF(args []string, w http.ResponseWriter, slashCommand *MMSlashCommand) error {
	if len(args) < 1 {
		return NewError("You need to specify a build tag. A branch or pr-0000.", nil)
	}

	if err := LoadtestKube(args[0]); err != nil {
		return err
	}

	WriteResponse(w, "Loadtesting: "+args[0])
	return nil
}
