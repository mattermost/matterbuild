// Copyright (c) 2018-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var client *github.Client
var ctx = context.Background()

func CreateMergeAndPr(branchToMerge string) (string, *AppError) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: Cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	var repoError []string
	var prs []string
	for _, repo := range Cfg.Repositories {
		if pr, err := createMergeAndPr(repo, branchToMerge); err != nil {
			LogError("Error while creating the merge: " + err.Error())
			repoError = append(repoError, err.Error())
		} else {
			prs = append(prs, pr)
		}
	}

	var msg string
	msg = "#### PRs:\n"
	for _, pr := range prs {
		msg = msg + "- " + pr + "\n"
	}

	msg = msg + "#### Errors:\n"
	for _, errMsg := range repoError {
		msg = msg + errMsg + "\n"
	}

	return msg, nil
}

func createMergeAndPr(repository *Repository, branchToMerge string) (string, *AppError) {
	refMaster := "refs/heads/master"

	branchRef := fmt.Sprintf("refs/heads/%s", branchToMerge)
	_, _, err := client.Git.GetRef(ctx, repository.Owner, repository.Name, branchRef)
	if err != nil {
		return "", NewError("Error when getting the release branch ref. Please check if that exists", err)
	}

	masterRef, _, err := client.Git.GetRef(ctx, repository.Owner, repository.Name, refMaster)
	if err != nil {
		return "", NewError("Error when getting the master ref.", err)
	}
	LogInfo("Master Ref: " + *masterRef.Object.SHA + " for repo: " + repository.Name)

	timeNow := time.Now().Format("20060102150405")
	newBranchName := fmt.Sprintf("refs/heads/merge-%s-%s", branchToMerge, timeNow)
	newBranch := &github.Reference{
		Ref: github.String(newBranchName),
		Object: &github.GitObject{
			SHA: masterRef.Object.SHA,
		},
	}

	newBranch, _, err = client.Git.CreateRef(ctx, repository.Owner, repository.Name, newBranch)
	if err != nil {
		return "", NewError("Error when creating the new branch.", err)
	}
	LogInfo("New Branch Ref: " + *newBranch.Ref + " for repo: " + repository.Name)

	commitMessage := fmt.Sprintf("Merge %s", branchToMerge)
	newMerge := &github.RepositoryMergeRequest{
		Base:          newBranch.Ref,
		Head:          github.String(branchToMerge),
		CommitMessage: github.String(commitMessage),
	}

	merge, _, err := client.Repositories.Merge(ctx, repository.Owner, repository.Name, newMerge)
	if err != nil {
		msg := fmt.Sprintf("Error when merging the branch. Please perform the merge manually for %s.", repository.Name)
		return "", NewError(msg, err)
	}
	LogInfo("Merge created: " + *merge.HTMLURL + " for repo: " + repository.Name)

	title := fmt.Sprintf("Merge %s-%s", branchToMerge, timeNow)
	prDescription := fmt.Sprintf("#### Summary \n Merge from `%s` to `master`", branchToMerge)
	newPR := &github.NewPullRequest{
		Title:               github.String(title),
		Head:                github.String(newBranchName),
		Base:                github.String(refMaster),
		Body:                github.String(prDescription),
		MaintainerCanModify: github.Bool(true),
	}

	pr, _, err := client.PullRequests.Create(ctx, repository.Owner, repository.Name, newPR)
	if err != nil {
		return "", NewError("Error when creating the PR.", err)
	}
	LogInfo("PR created: " + pr.GetHTMLURL() + " for repo: " + repository.Name)

	return pr.GetHTMLURL(), nil
}
