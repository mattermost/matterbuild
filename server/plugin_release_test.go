// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/matterbuild/server/mocks"
)

func TestCreatePlatformPlugins(t *testing.T) {
	t.Run("invalid plugin file", func(t *testing.T) {
		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		platformPluginFilePaths, err := createPlatformPlugins("myrepo", "mytag", "invalid", tmpFolder)
		require.Error(t, err)
		require.Nil(t, platformPluginFilePaths)
	})

	t.Run("plugin tar has all platform binaries", func(t *testing.T) {
		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz")

		expectedFiles := map[string]string{
			"myrepo-mytag-darwin-amd64.tar.gz":  "plugin-darwin-amd64",
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
		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-demo-v0.4.1-linux-amd64.tar.gz")

		expectedFiles := map[string]string{
			"myrepo-mytag-linux-amd64.tar.gz": "plugin-linux-amd64",
		}
		platformPluginFilePaths, err := createPlatformPlugins("myrepo", "mytag", path, tmpFolder)
		require.NoError(t, err)
		require.Len(t, platformPluginFilePaths, 1)

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

	t.Run("plugin tar only has amd64 binaries (missing arm64)", func(t *testing.T) {
		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-demo-v0.4.1-amd64.tar.gz")

		expectedFiles := map[string]string{
			"myrepo-mytag-darwin-amd64.tar.gz":  "plugin-darwin-amd64",
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

	t.Run("calls plugin tar has only two platform binaries", func(t *testing.T) {
		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		path := filepath.Join("test", "mattermost-plugin-calls-v0.5.0.tar.gz")

		expectedFiles := map[string]string{
			"mattermost-plugin-calls-mytag-linux-amd64.tar.gz":   "plugin-linux-amd64",
			"mattermost-plugin-calls-mytag-freebsd-amd64.tar.gz": "plugin-freebsd-amd64",
		}
		platformPluginFilePaths, err := createPlatformPlugins("mattermost-plugin-calls", "mytag", path, tmpFolder)
		require.NoError(t, err)
		require.Len(t, platformPluginFilePaths, 2)

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
}

func TestArchiveContains(t *testing.T) {
	t.Run("invalid archive file", func(t *testing.T) {
		found, err := archiveContains("invalid", "mytag")
		require.Error(t, err)
		require.Nil(t, found)
	})

	t.Run("archive returns correct strings", func(t *testing.T) {
		found, err := archiveContains(filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz"), "plugin-")
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Len(t, found, 5)
		require.Contains(t, found, "plugin-darwin-amd64")
		require.Contains(t, found, "plugin-darwin-arm64")
		require.Contains(t, found, "plugin-windows-amd64.exe")
		require.Contains(t, found, "plugin-linux-amd64")
		require.Contains(t, found, "plugin-linux-arm64")
	})

	t.Run("archive should only match filenames and not full path", func(t *testing.T) {
		found, err := archiveContains(filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz"), "plugin")
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Len(t, found, 6)
		require.Contains(t, found, "plugin-darwin-amd64")
		require.Contains(t, found, "plugin-darwin-arm64")
		require.Contains(t, found, "plugin-windows-amd64.exe")
		require.Contains(t, found, "plugin-linux-amd64")
		require.Contains(t, found, "plugin-linux-arm64")
		require.Contains(t, found, "plugin.json")
	})

	t.Run("archive returns no strings", func(t *testing.T) {
		found, err := archiveContains(filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz"), "meow")
		require.NoError(t, err)
		require.Len(t, found, 0)
	})
}

func TestFindlatformBinaries(t *testing.T) {
	t.Run("invalid archive file", func(t *testing.T) {
		platformBinaries, err := findPlatformBinaries("invalid")
		require.Error(t, err)
		require.Empty(t, platformBinaries)
	})

	t.Run("missing two platform binaries", func(t *testing.T) {
		platformBinaries, err := findPlatformBinaries(filepath.Join("test", "mattermost-plugin-demo-v0.4.1-linux-amd64.tar.gz"))
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"linux-amd64": "plugin-linux-amd64",
		}, platformBinaries)
	})

	t.Run("contains all platform binaries", func(t *testing.T) {
		platformBinaries, err := findPlatformBinaries(filepath.Join("test", "mattermost-plugin-demo-v0.4.1.tar.gz"))
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"darwin-amd64":  "plugin-darwin-amd64",
			"windows-amd64": "plugin-windows-amd64.exe",
			"linux-amd64":   "plugin-linux-amd64",
		}, platformBinaries)
	})
}

func TestCreateTag(t *testing.T) {
	t.Run("create tag using master's tip", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		gitMock := mocks.NewMockGithubGitService(ctrl)
		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		commitSHA := ""

		testClient := &GithubClient{
			Git:          gitMock,
			Repositories: repoMock,
		}
		gitMock.EXPECT().GetRefs(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(fmt.Sprintf("tags/%s", tag))).Return(nil, nil, nil)

		repo := &github.Repository{
			DefaultBranch: github.String("master"),
		}
		repoMock.EXPECT().Get(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName)).Return(repo, nil, nil)

		masterRef := &github.Reference{
			Object: &github.GitObject{
				SHA: github.String("master-SHA"),
			},
		}
		gitMock.EXPECT().GetRef(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq("heads/master")).Return(masterRef, nil, nil)

		githubObj := &github.GitObject{
			SHA:  masterRef.Object.SHA,
			Type: github.String("commit"),
		}
		githubTag := &github.Tag{
			Tag:     github.String(tag),
			Message: github.String(tag),
			Object:  githubObj,
		}
		gitMock.EXPECT().CreateTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(githubTag)).Return(nil, nil, nil)

		refTag := &github.Reference{
			Ref:    github.String(fmt.Sprintf("tags/%s", tag)),
			Object: githubObj,
		}
		gitMock.EXPECT().CreateRef(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(refTag)).Return(nil, nil, nil)

		err := createTag(ctx, testClient, owner, repoName, tag, commitSHA)
		require.NoError(t, err)
	})

	t.Run("create tag using given commit SHA", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		gitMock := mocks.NewMockGithubGitService(ctrl)
		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		commitSHA := "sha"

		testClient := &GithubClient{
			Git:          gitMock,
			Repositories: repoMock,
		}
		gitMock.EXPECT().GetRefs(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(fmt.Sprintf("tags/%s", tag))).Return(nil, nil, nil)

		githubObj := &github.GitObject{
			SHA:  github.String(commitSHA),
			Type: github.String("commit"),
		}
		githubTag := &github.Tag{
			Tag:     github.String(tag),
			Message: github.String(tag),
			Object:  githubObj,
		}
		gitMock.EXPECT().CreateTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(githubTag)).Return(nil, nil, nil)
		repoMock.EXPECT().GetCommit(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(commitSHA)).Return(nil, nil, nil)

		refTag := &github.Reference{
			Ref:    github.String(fmt.Sprintf("tags/%s", tag)),
			Object: githubObj,
		}
		gitMock.EXPECT().CreateRef(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(refTag)).Return(nil, nil, nil)

		err := createTag(ctx, testClient, owner, repoName, tag, commitSHA)
		require.NoError(t, err)
	})

	t.Run("create tag that returns other matching tags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		gitMock := mocks.NewMockGithubGitService(ctrl)
		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		commitSHA := "sha"

		testClient := &GithubClient{
			Git:          gitMock,
			Repositories: repoMock,
		}
		refs := []*github.Reference{
			{
				Ref: github.String("tags/testTag-1"),
			},
			{
				Ref: github.String("tags/testTag-2"),
			},
		}

		gitMock.EXPECT().GetRefs(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(fmt.Sprintf("tags/%s", tag))).Return(refs, nil, nil)

		githubObj := &github.GitObject{
			SHA:  github.String(commitSHA),
			Type: github.String("commit"),
		}
		githubTag := &github.Tag{
			Tag:     github.String(tag),
			Message: github.String(tag),
			Object:  githubObj,
		}
		gitMock.EXPECT().CreateTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(githubTag)).Return(nil, nil, nil)
		repoMock.EXPECT().GetCommit(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(commitSHA)).Return(nil, nil, nil)

		refTag := &github.Reference{
			Ref:    github.String(fmt.Sprintf("tags/%s", tag)),
			Object: githubObj,
		}
		gitMock.EXPECT().CreateRef(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(refTag)).Return(nil, nil, nil)

		err := createTag(ctx, testClient, owner, repoName, tag, commitSHA)
		require.NoError(t, err)
	})

	t.Run("create tag that returns matching tags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		gitMock := mocks.NewMockGithubGitService(ctrl)
		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		commitSHA := "sha"

		testClient := &GithubClient{
			Git:          gitMock,
			Repositories: repoMock,
		}
		refs := []*github.Reference{
			{
				Ref: github.String("tags/testTag-1"),
			},
			{
				Ref: github.String("tags/testTag-2"),
			},
			{
				Ref: github.String("tags/testTag"),
			},
		}

		gitMock.EXPECT().GetRefs(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(fmt.Sprintf("tags/%s", tag))).Return(refs, nil, nil)

		err := createTag(ctx, testClient, owner, repoName, tag, commitSHA)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrTagExists))
	})
}

func TestGetPluginAsset(t *testing.T) {
	ctx := context.Background()
	release := &github.RepositoryRelease{}

	t.Run("should find the tarball if only one exists", func(t *testing.T) {
		release.Assets = []github.ReleaseAsset{
			{ID: github.Int64(1), Name: github.String("README.txt")},
			{ID: github.Int64(2), Name: github.String("tarball.tar.gz")},
		}

		asset, err := getPluginAsset(ctx, release, "")
		require.NoError(t, err)
		require.Equal(t, github.String("tarball.tar.gz"), asset.Name)
	})

	t.Run("should error if more than one tarball exists", func(t *testing.T) {
		release.Assets = []github.ReleaseAsset{
			{ID: github.Int64(1), Name: github.String("tarball.tar.gz")},
			{ID: github.Int64(2), Name: github.String("plugin-tarball.tar.gz")},
		}

		asset, err := getPluginAsset(ctx, release, "")
		require.EqualError(t, err, "found unexpected file plugin-tarball.tar.gz")
		require.Nil(t, asset)
	})

	t.Run("should find a specific asset if a name is passed", func(t *testing.T) {
		release.Assets = []github.ReleaseAsset{
			{ID: github.Int64(1), Name: github.String("tarball.tar.gz")},
			{ID: github.Int64(2), Name: github.String("plugin-tarball.tar.gz")},
		}

		asset, err := getPluginAsset(ctx, release, "plugin-tarball.tar.gz")
		require.NoError(t, err)
		require.Equal(t, github.String("plugin-tarball.tar.gz"), asset.Name)
	})
}

func TestDownloadAsset(t *testing.T) {
	t.Run("should error if nothing is downloaded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"

		testClient := &GithubClient{
			Repositories: repoMock,
		}
		asset := &github.ReleaseAsset{
			ID:   github.Int64(5),
			Name: github.String("test_asset"),
		}
		repoMock.EXPECT().DownloadReleaseAsset(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(asset.GetID())).Return(nil, "", nil)

		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		assetFilePath, err := downloadAsset(ctx, testClient, owner, repoName, asset, tmpFolder)
		require.Error(t, err)
		require.Empty(t, assetFilePath)
	})

	t.Run("download github release asset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"

		testClient := &GithubClient{
			Repositories: repoMock,
		}
		asset := &github.ReleaseAsset{
			ID:   github.Int64(5),
			Name: github.String("test_asset"),
		}
		expectedData := "hello world"
		rc := io.NopCloser(bytes.NewReader([]byte(expectedData)))
		repoMock.EXPECT().DownloadReleaseAsset(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(asset.GetID())).Return(rc, "", nil)

		tmpFolder, err := os.MkdirTemp("", "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		assetFilePath, err := downloadAsset(ctx, testClient, owner, repoName, asset, tmpFolder)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(tmpFolder, "test_asset"), assetFilePath)

		data, err := os.ReadFile(assetFilePath)
		require.NoError(t, err)
		require.Equal(t, expectedData, string(data))
	})
}

func TestGetSuccessMessage(t *testing.T) {
	repo := "mattermost-plugin-jira"
	tag := "v3.0.0"
	commitSHA := "8ba315752a0ea59d319f71b71fb8c5cb6f353f77"
	releaseURL := "https://github.com/mattermost/mattermost-plugin-jira/releases/tag/v3.0.0"
	username := "foo"

	actualMessage := getSuccessMessage(tag, repo, commitSHA, releaseURL, username)
	expectedMessage := `@foo A Plugin was successfully signed and uploaded to Github and S3.
Tag: **v3.0.0**
Repo: **mattermost-plugin-jira**
CommitSHA: **8ba315752a0ea59d319f71b71fb8c5cb6f353f77**
[Release Link](https://github.com/mattermost/mattermost-plugin-jira/releases/tag/v3.0.0)
To add this release to the Plugin Marketplace run inside your local Marketplace repository:` + "\n```\n" +
		`git checkout production
git pull
git checkout -b add_mattermost-plugin-jira_v3.0.0
go run ./cmd/generator/ add mattermost-plugin-jira v3.0.0 [--official|--community] [--beta] [--enterprise]` + "\n```\n" +
		"Use `--official` for plugins maintained by Mattermost and `--community` for ones maintained by the Open Source community.\n" +
		"You might want to use other flag like `--beta` to add a `Beta` label, or `--enterprise` for plugins that require an E20 license.\n" +
		"\n" +
		"Then review your changes by running `git diff plugins.json`" + "\n```\n" +
		`git commit plugins.json -m "Add v3.0.0 of mattermost-plugin-jira to the Marketplace"
git push --set-upstream origin add_mattermost-plugin-jira_v3.0.0
git checkout master` + "\n```\n" +
		`Use https://github.com/mattermost/mattermost-marketplace/compare/production...add_mattermost-plugin-jira_v3.0.0?quick_pull=1&labels=3:+QA+Review,2:+Dev+Review to open a Pull Request.`

	assert.Equal(t, expectedMessage, actualMessage)
}

func TestMarkTagAsPreRelease(t *testing.T) {
	t.Run("failed to get release by tag", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		expectedErr := errors.New("test error on getting release by tag")

		testClient := &GithubClient{
			Repositories: repoMock,
		}

		repoMock.EXPECT().GetReleaseByTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(tag)).Return(nil, nil, expectedErr)

		err := markTagAsPreRelease(ctx, testClient, owner, repoName, tag)
		require.Error(t, err)
	})

	t.Run("failed to edit release", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		var releaseID int64 = 42
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		release := &github.RepositoryRelease{ID: &releaseID}
		expectedErr := errors.New("test error on editing release")
		preRelease := true

		testClient := &GithubClient{
			Repositories: repoMock,
		}

		repoMock.EXPECT().GetReleaseByTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(tag)).Return(release, nil, nil)
		repoMock.EXPECT().EditRelease(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(release.GetID()), gomock.Eq(&github.RepositoryRelease{Prerelease: &preRelease})).Return(nil, nil, expectedErr)

		err := markTagAsPreRelease(ctx, testClient, owner, repoName, tag)
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := context.Background()

		repoMock := mocks.NewMockGithubRepositoriesService(ctrl)
		var releaseID int64 = 42
		owner := "owner"
		repoName := "repoName"
		tag := "testTag"
		release := &github.RepositoryRelease{ID: &releaseID}
		preRelease := true

		testClient := &GithubClient{
			Repositories: repoMock,
		}

		repoMock.EXPECT().GetReleaseByTag(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(tag)).Return(release, nil, nil)
		repoMock.EXPECT().EditRelease(gomock.Eq(ctx), gomock.Eq(owner), gomock.Eq(repoName), gomock.Eq(release.GetID()), gomock.Eq(&github.RepositoryRelease{Prerelease: &preRelease})).Return(nil, nil, nil)

		err := markTagAsPreRelease(ctx, testClient, owner, repoName, tag)
		require.NoError(t, err)
	})
}
