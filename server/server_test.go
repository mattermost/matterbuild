// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
