// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginTagRegex(t *testing.T) {
	t.Run("valid tags", func(t *testing.T) {
		require.Regexp(t, regexp.MustCompile(pluginTagRegex), "v5.2.3")
		require.Regexp(t, regexp.MustCompile(pluginTagRegex), "v55.52.53")
		require.Regexp(t, regexp.MustCompile(pluginTagRegex), "v552.522.532")
	})

	t.Run("invalid tags", func(t *testing.T) {
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "5.2.3")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "5a2.3")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "5.2a3")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), " v 5 . 2 . 3 ")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "v55..52.53")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "v552.522..532")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "zzzv552.522.532")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "123zzzv552.522.532")
		require.NotRegexp(t, regexp.MustCompile(pluginTagRegex), "    v552.522.532    ")
	})
}

func TestCheckSlashPermissions(t *testing.T) {
	t.Run("allowed commands", func(t *testing.T) {
		Cfg = &MatterbuildConfig{
			AllowedTokens: []string{"token"},
			AllowedUsers:  []string{"userid1", "userid2"},
			ReleaseUsers:  []string{"userid1", "userid3"},
		}
		commands := []*MMSlashCommand{
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid1", Text: "cut 0.0.0-rc0"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid1", Text: "cutplugin --tag v0.0.0-rc0 --repo testplugin"},
		}
		rootCmd := initCommands(nil, nil)
		for _, command := range commands {
			require.Nil(t, checkSlashPermissions(command, rootCmd))
		}
	})

	t.Run("disallowed commands", func(t *testing.T) {
		Cfg = &MatterbuildConfig{
			AllowedTokens: []string{"token"},
			AllowedUsers:  []string{"userid1", "userid2"},
			ReleaseUsers:  []string{"userid1", "userid3"},
		}
		commands := []*MMSlashCommand{
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid2", Text: "cut 0.0.0-rc0"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid3", Text: "cut 0.0.0-rc0"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid4", Text: "cut 0.0.0-rc0"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid2", Text: "cutplugin --tag v0.0.0-rc0 --repo testplugin"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid3", Text: "cutplugin --tag v0.0.0-rc0 --repo testplugin"},
			&MMSlashCommand{Command: "/matterbuild", Token: "token", UserId: "userid4", Text: "cutplugin --tag v0.0.0-rc0 --repo testplugin"},
		}
		rootCmd := initCommands(nil, nil)
		for _, command := range commands {
			require.NotNil(t, checkSlashPermissions(command, rootCmd))
		}
	})
}
