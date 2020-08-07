// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/bndr/gojenkins"
	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
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
	ResponseUrl string `schema:"response_url"`
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
	w.Write(GenerateEnrichedSlashResponse(title, resp, color, style))
}

func PostExtraMessages(responseURL string, payload []byte) error {
	req, err := http.NewRequest("POST", responseURL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
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

type Config struct {
	SSLVerify bool
	CACrtPath bool
}

var config = &Config{}

func Start() {
	LoadConfig("config.json")
	LogInfo("Starting Matterbuild")

	flag.BoolVar(&config.SSLVerify, "ssl-verify", true, "Verify Jenkins SSL")
	flag.BoolVar(&config.CACrtPath, "ca-cert", true, "Use Jenkins CA certificate")
	flag.Parse()

	router := httprouter.New()
	router.GET("/", indexHandler)
	router.GET("/healthz", healthHandler)
	router.POST("/slash_command", slashCommandHandler)

	LogInfo("Running Matterbuild on port " + Cfg.ListenAddress)
	if err := http.ListenAndServe(Cfg.ListenAddress, router); err != nil {
		LogError(err.Error())
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Write([]byte("This is the matterbuild server."))
}
func healthHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	w.Write([]byte("Healthy!"))
}

func checkSlashPermissions(command *MMSlashCommand, rootCmd *cobra.Command) *AppError {
	hasPermissions := false
	for _, allowedToken := range Cfg.AllowedTokens {
		if allowedToken == command.Token {
			hasPermissions = true
			break
		}
	}

	if !hasPermissions {
		return NewError("Token for slash command is incorrect", nil)
	}

	hasPermissions = false
	for _, allowedUser := range Cfg.AllowedUsers {
		if allowedUser == command.UserId {
			hasPermissions = true
			break
		}
	}

	if !hasPermissions {
		return NewError("You don't have permissions to use this command.", nil)
	}

	subCommand, _, _ := rootCmd.Find(strings.Fields(strings.TrimSpace(command.Text)))
	if subCommand.Name() == "cut" || subCommand.Name() == "cutplugin" {
		hasPermissions = false
		for _, allowedUser := range Cfg.ReleaseUsers {
			if allowedUser == command.UserId {
				hasPermissions = true
				break
			}
		}

		if !hasPermissions {
			return NewError("You don't have permissions to use this command.", nil)
		}
	}

	return nil
}

func initCommands(w http.ResponseWriter, command *MMSlashCommand) *cobra.Command {
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

	var cutPluginCmd = &cobra.Command{
		Use:   "cutplugin [--tag] [--repo] [--commitSHA] [--force]",
		Short: "Cut a release of any plugin under Mattermost Organization",
		Long:  "Cut a release of any plugin under Mattermost Organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			repo, _ := cmd.Flags().GetString("repo")
			commitSHA, _ := cmd.Flags().GetString("commitSHA")
			force, _ := cmd.Flags().GetBool("force")
			return cutPluginCommandF(w, command, tag, repo, commitSHA, force)
		},
	}
	cutPluginCmd.Flags().String("tag", "", "Set this flag for the tag you want to release.")
	cutPluginCmd.Flags().String("repo", "", "Set this flag for the plugin repository.")
	cutPluginCmd.Flags().String("commitSHA", "", "Set this flag for the commit you want to use for the tag. Defaults to master's tip.")
	cutPluginCmd.Flags().Bool("force", false, "Set this flag to regenerate assets for a given repository.")

	var setCIBranchCmd = &cobra.Command{
		Use:   "setci",
		Short: "Set the branch target for the CI servers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setCIBranchCmdF(args, w, command)
		},
	}

	var setLatestReleaseURLCmd = &cobra.Command{
		Use:   "latestURL [--typeToRelease] [--ver]",
		Short: "Set the latest URLs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			typeToRelease, _ := cmd.Flags().GetString("typeToRelease")
			releaseVer, _ := cmd.Flags().GetString("ver")
			return setLatestReleaseURLCmdF(w, command, typeToRelease, releaseVer)
		},
	}
	setLatestReleaseURLCmd.Flags().String("typeToRelease", "", "Set the type of release - server, desktop or both.")
	setLatestReleaseURLCmd.Flags().String("ver", "", "Set the version number to use.")

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

	rootCmd.AddCommand(cutCmd, configDumpCmd, setCIBranchCmd, runJobCmd, checkCutReleaseStatusCmd, lockTranslationServerCmd, checkBranchTranslationCmd, cutPluginCmd, setLatestReleaseURLCmd)

	return rootCmd
}

func slashCommandHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	command, err := ParseSlashCommand(r)
	if err != nil {
		WriteErrorResponse(w, NewError("Unable to parse incoming slash command info", err))
		return
	}

	rootCmd := initCommands(w, command)

	if err := checkSlashPermissions(command, rootCmd); err != nil {
		WriteErrorResponse(w, err)
		return
	}

	// Output Buffer
	outBuf := &bytes.Buffer{}

	rootCmd.SetArgs(strings.Fields(strings.TrimSpace(command.Text)))
	rootCmd.SetOutput(outBuf)

	err = rootCmd.Execute()

	if err != nil || len(outBuf.String()) > 0 {
		WriteEnrichedResponse(w, "Information", outBuf.String(), "#0060aa", EPHEMERAL)
	}
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

		// If this is a full release update the latest URLs
		if rcPart == "" {
			typeToRelease := "server"
			if webapp != "" {
				typeToRelease = "desktop"
			}

			if err := SetLatestURL(typeToRelease, releasePart, Cfg); err != nil {
				LogError("Error when setting the latest URLs. err= " + err.Error())
				return err
			}
			msg := fmt.Sprintf("Latest URLs for %s will also be updated to version: %s", typeToRelease, releasePart)
			WriteEnrichedResponse(w, "Cut Release", msg, "#0060aa", IN_CHANNEL)
		}
	}

	return nil
}

func cutPluginCommandF(w http.ResponseWriter, slashCommand *MMSlashCommand, tag, repo, commitSHA string, force bool) error {
	if tag == "" {
		WriteErrorResponse(w, NewError("Tag should not be empty", nil))
		return nil
	}
	if tag[0] != 'v' {
		WriteErrorResponse(w, NewError("Tag must start with leading 'v'", nil))
		return nil
	}

	if _, err := semver.Parse(tag[1:]); err != nil {
		WriteErrorResponse(w, NewError(fmt.Sprintf("Tag must adhere to semver after leading 'v': %s", err.Error()), nil))
		return nil
	}

	if repo == "" {
		WriteErrorResponse(w, NewError("Plugin Repository should not be empty", nil))
		return nil
	}

	ctx := context.Background()
	client := NewGithubClient(ctx, Cfg.GithubAccessToken)
	if err := checkRepo(ctx, client, Cfg.GithubOrg, repo); err != nil {
		WriteErrorResponse(w, NewError(err.Error(), nil))
		return nil
	}

	command := slashCommand.Command + "" + slashCommand.Text
	msg := fmt.Sprintf("@%s triggered a plugin release process using `%s`.\nTag %s created in  `%s`. Waiting for the artifacts to sign and publish.\nWill report back when the process completes.\nGrab :coffee: and a :doughnut: ", slashCommand.Username, command, tag, repo)
	if err := createTag(ctx, client, Cfg.GithubOrg, repo, tag, commitSHA); errors.Is(err, ErrTagExists) {
		if !force {
			WriteErrorResponse(w, NewError(fmt.Errorf("@%s Tag %s already exists in %s. Not generating any artifacts. Use --force to regenerate artifacts.", slashCommand.Username, repo, tag).Error(), nil))
			return nil
		}
		msg = fmt.Sprintf("@%s Tag %s already exists in %s. Waiting for the artifacts to sign and publish.\nWill report back when the process completes.\nGrab :coffee: and a :doughnut: ", slashCommand.Username, tag, repo)
	} else if err != nil {
		WriteErrorResponse(w, NewError(err.Error(), nil))
		return nil
	}

	WriteEnrichedResponse(w, "Plugin Release Process", msg, "#0060aa", IN_CHANNEL)

	go func() {
		if err := cutPlugin(ctx, Cfg, client, Cfg.GithubOrg, repo, tag); err != nil {
			LogError("failed to cutplugin %s", err.Error())
			errMsg := fmt.Sprintf("Error while signing plugin\nError: %s", err.Error())
			errColor := "#fc081c"
			if err := PostExtraMessages(slashCommand.ResponseUrl, GenerateEnrichedSlashResponse("Plugin Release Process", errMsg, errColor, IN_CHANNEL)); err != nil {
				LogError("failed to post err through PostExtraMessages err=%s", err.Error())
			}
			return
		}

		// Get release link if possible
		releaseURL := ""
		if release, err := getReleaseByTag(ctx, client, Cfg.GithubOrg, repo, tag); err != nil {
			LogError("failed to get release by tag after err=%s", err.Error())
		} else {
			releaseURL = release.GetHTMLURL()
		}

		branch := fmt.Sprintf("add_%s_%s", repo, tag)

		// Purposefully left unindented to avoid introducing extra spaces when posting the message.
		marketplaceCommand := fmt.Sprintf(`
git checkout production
git pull
git checkout -b %[3]s
make plugins.json
make generate
git commit plugins.json data/statik/statik.go -m "Add %[1]s of %[2]s to the Marketplace"
git push --set-upstream origin %[3]s
git checkout master
`, tag, repo, branch)

		url := fmt.Sprintf(
			"https://github.com/mattermost/mattermost-marketplace/compare/production...%s?quick_pull=1&labels=3:+QA+Review,2:+Dev+Review",
			branch,
		)
		if commitSHA != "" {
			msg = fmt.Sprintf(
				"@%s A Plugin was successfully signed and uploaded to Github and S3.\nTag: **%s**\nRepo: **%s**\nCommitSHA: **%s**\n[Release Link](%s)\nTo add this release to the Plugin Marketplace run inside your local Marketplace repository:\n```%s\n```\nUse %s to open a Pull Request.",
				slashCommand.Username, tag, repo, commitSHA, releaseURL, marketplaceCommand, url,
			)
		} else {
			msg = fmt.Sprintf(
				"@%s A Plugin was successfully signed and uploaded to Github and S3.\nTag: **%s**\nRepo: **%s**\n[Release Link](%s)\nTo add this release to the Plugin Marketplace run inside your local Marketplace repository:\n```%s\n```\nUse %s to open a Pull Request.",
				slashCommand.Username, tag, repo, releaseURL, marketplaceCommand, url,
			)
		}

		color := "#0060aa"
		if err := PostExtraMessages(slashCommand.ResponseUrl, GenerateEnrichedSlashResponse("Plugin Release Process", msg, color, IN_CHANNEL)); err != nil {
			LogError("failed to post success msg through PostExtraMessages err=%s", err.Error())
		}
	}()
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

func setLatestReleaseURLCmdF(w http.ResponseWriter, slashCommand *MMSlashCommand, typeToRelease string, ver string) error {

	if typeToRelease == "" || ver == "" {
		WriteErrorResponse(w, NewError("Need to define which of the latest URLs should be updated and what version string to use", nil))
		return nil
	}

	if err := SetLatestURL(typeToRelease, ver, Cfg); err != nil {
		LogError("Error when setting the latest URLs. err= " + err.Error())
		return err
	}

	LogInfo("Latest %s URLS now updated to: %s", typeToRelease, ver)

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
