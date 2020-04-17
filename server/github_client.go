// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"context"
	"io"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type GithubRepositoriesService interface {
	GetCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, *github.Response, error)
	ListTags(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryTag, *github.Response, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error)
	ListReleaseAssets(ctx context.Context, owner, repo string, id int64, opt *github.ListOptions) ([]*github.ReleaseAsset, *github.Response, error)
	DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (rc io.ReadCloser, redirectURL string, err error)
	UploadReleaseAsset(ctx context.Context, owner, repo string, id int64, opt *github.UploadOptions, file *os.File) (*github.ReleaseAsset, *github.Response, error)
	DeleteReleaseAsset(ctx context.Context, owner, repo string, id int64) (*github.Response, error)
}

type GithubSearchService interface {
	Repositories(ctx context.Context, query string, opt *github.SearchOptions) (*github.RepositoriesSearchResult, *github.Response, error)
}

type GithubGitService interface {
	GetRef(ctx context.Context, owner string, repo string, ref string) (*github.Reference, *github.Response, error)
	GetRefs(ctx context.Context, owner string, repo string, ref string) ([]*github.Reference, *github.Response, error)
	CreateTag(ctx context.Context, owner string, repo string, tag *github.Tag) (*github.Tag, *github.Response, error)
	CreateRef(ctx context.Context, owner string, repo string, ref *github.Reference) (*github.Reference, *github.Response, error)
}

// GithubClient wraps the github.Client with relevant interfaces.
type GithubClient struct {
	Repositories GithubRepositoriesService
	Search       GithubSearchService
	Git          GithubGitService
}

func NewGithubClient(ctx context.Context, accessToken string) *GithubClient {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &GithubClient{
		Repositories: client.Repositories,
		Search:       client.Search,
		Git:          client.Git,
	}
}
