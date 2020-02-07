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
