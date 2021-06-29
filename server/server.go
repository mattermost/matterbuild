// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/blang/semver"
	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/mattermost/matterbuild/version"
)

type MMSlashCommand struct {
	ChannelID   string `schema:"channel_id"`
	ChannelName string `schema:"channel_name"`
	Command     string `schema:"command"`
	TeamName    string `schema:"team_domain"`
	TeamID      string `schema:"team_id"`
	Text        string `schema:"text"`
	Token       string `schema:"token"`
	UserID      string `schema:"user_id"`
	Username    string `schema:"user_name"`
	ResponseURL string `schema:"response_url"`
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

func WriteErrorResponse(w http.ResponseWriter, err *AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(GenerateStandardSlashResponse(err.Error(), model.COMMAND_RESPONSE_TYPE_EPHEMERAL)))
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
	req, err := http.NewRequest(http.MethodPost, responseURL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

type healthResponse struct {
	Info *version.Info `json:"info"`
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
	err := json.NewEncoder(w).Encode(healthResponse{Info: version.Full()})
	if err != nil {
		LogError(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
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
		if allowedUser == command.UserID {
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
			if allowedUser == command.UserID {
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

	var cutPluginCmd = &cobra.Command{
		Use:   "cutplugin [--tag] [--repo] [--commitSHA] [--force] [--pre-release]",
		Short: "Cut a release of any plugin under Mattermost Organization",
		Long:  "Cut a release of any plugin under Mattermost Organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			repo, _ := cmd.Flags().GetString("repo")
			commitSHA, _ := cmd.Flags().GetString("commitSHA")
			assetName, _ := cmd.Flags().GetString("asset-name")
			force, _ := cmd.Flags().GetBool("force")
			preRelease, _ := cmd.Flags().GetBool("pre-release")
			return cutPluginCommandF(w, command, tag, repo, commitSHA, assetName, force, preRelease)
		},
	}
	cutPluginCmd.Flags().String("tag", "", "Set this flag for the tag you want to release.")
	cutPluginCmd.Flags().String("repo", "", "Set this flag for the plugin repository.")
	cutPluginCmd.Flags().String("commitSHA", "", "Set this flag for the commit you want to use for the tag. Defaults to master's tip.")
	cutPluginCmd.Flags().String("asset-name", "", "Set this flag for the file name of the asset to sign. Defaults to the asset with `.tar.gz` extension.")
	cutPluginCmd.Flags().Bool("force", false, "Set this flag to regenerate assets for a given repository.")
	cutPluginCmd.Flags().Bool("pre-release", false, "Set this flag to label this version as pre-release.")

	rootCmd.AddCommand(cutPluginCmd)

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
		WriteEnrichedResponse(w, "Information", outBuf.String(), "#0060aa", model.COMMAND_RESPONSE_TYPE_EPHEMERAL)
	}
}

func cutPluginCommandF(w http.ResponseWriter, slashCommand *MMSlashCommand, tag, repo, commitSHA, assetName string, force bool, preRelease bool) error {
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

	releasePrefix := ""
	if preRelease {
		releasePrefix = "pre-"
	}
	command := slashCommand.Command + " " + slashCommand.Text
	msg := fmt.Sprintf("@%s triggered a plugin %srelease process using `%s`.\nTag %s created in`%s`. Waiting for the artifacts to sign and publish.\nWill report back when the process completes.\nGrab :coffee: and a :doughnut: ", slashCommand.Username, releasePrefix, command, tag, repo)
	if err := createTag(ctx, client, Cfg.GithubOrg, repo, tag, commitSHA); errors.Is(err, ErrTagExists) {
		if !force {
			WriteErrorResponse(w, NewError(fmt.Sprintf("@%s Tag %s already exists in %s. Not generating any artifacts. Use --force to regenerate artifacts.", slashCommand.Username, tag, repo), nil))
			return nil
		}
		msg = fmt.Sprintf("@%s Tag %s already exists in %s. Waiting for the artifacts to sign and publish.\nWill report back when the process completes.\nGrab :coffee: and a :doughnut: ", slashCommand.Username, tag, repo)
	} else if err != nil {
		WriteErrorResponse(w, NewError(err.Error(), nil))
		return nil
	}

	WriteEnrichedResponse(w, "Plugin Release Process", msg, "#0060aa", model.COMMAND_RESPONSE_TYPE_IN_CHANNEL)

	go func() {
		if err := cutPlugin(ctx, Cfg, client, Cfg.GithubOrg, repo, tag, assetName, preRelease); err != nil {
			LogError("failed to cutplugin %s", err.Error())
			errMsg := fmt.Sprintf("Error while signing plugin\nError: %s", err.Error())
			errColor := "#fc081c"
			if err := PostExtraMessages(slashCommand.ResponseURL, GenerateEnrichedSlashResponse("Plugin Release Process", errMsg, errColor, model.COMMAND_RESPONSE_TYPE_IN_CHANNEL)); err != nil {
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

		msg := getSuccessMessage(tag, repo, commitSHA, releaseURL, slashCommand.Username)

		color := "#0060aa"
		if err := PostExtraMessages(slashCommand.ResponseURL, GenerateEnrichedSlashResponse("Plugin Release Process", msg, color, model.COMMAND_RESPONSE_TYPE_IN_CHANNEL)); err != nil {
			LogError("failed to post success msg through PostExtraMessages err=%s", err.Error())
		}
	}()
	return nil
}
