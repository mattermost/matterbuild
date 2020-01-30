// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreatePlatformPlugins(t *testing.T) {
	t.Run("invalid plugin file", func(t *testing.T) {
		tmpFolder, err := ioutil.TempDir("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		platformPluginFilePaths, err := createPlatformPlugins("myrepo", "mytag", "invalid", tmpFolder)
		require.NoError(t, err)
		require.Nil(t, platformPluginFilePaths)
	})

	t.Run("plugin tar has all platform binaries", func(t *testing.T) {
		tmpFolder, err := ioutil.TempDir("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz")

		expectedFiles := map[string]string{
			"myrepo-mytag-osx-amd64.tar.gz":     "plugin-darwin-amd64",
			"myrepo-mytag-windows-amd64.tar.gz": "plugin-windows-amd64.exe",
			"myrepo-mytag-linux-amd64.tar.gz":   "plugin-linux-amd64",
		}
		platformPluginFilePaths, err := createPlatformPlugins("myrepo", "mytag", path, tmpFolder)
		require.NoError(t, err)
		require.Len(t, platformPluginFilePaths, 3)

		for _, filePath := range platformPluginFilePaths {
			base := filepath.Base(filePath)
			require.Contains(t, expectedFiles, base)

			found, err := archiveContains(filePath, "plugin-")
			require.NoError(t, err)
			require.Len(t, found, 1)
			require.Equal(t, expectedFiles[base], found[0])
			delete(expectedFiles, base)
		}
		require.Len(t, expectedFiles, 0)
	})

	t.Run("linux plugin tar doesn't have all platform binaries", func(t *testing.T) {
		tmpFolder, err := ioutil.TempDir("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-demo-v0.4.1-linux-amd64.tar.gz")

		platformPluginFilePaths, err := createPlatformPlugins("myrepo", "mytag", path, tmpFolder)
		require.Error(t, err)
		require.Contains(t, err.Error(), "plugin-linux-amd64")
		require.NotContains(t, err.Error(), "plugin-windows-amd64")
		require.NotContains(t, err.Error(), "plugin-osx-amd64")
		require.Nil(t, platformPluginFilePaths)
	})
}
