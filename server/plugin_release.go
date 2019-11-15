// Copyright (c) 2018-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"bufio"
	// "bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"

	scp "github.com/cpanato/go-scp"
	"github.com/cpanato/go-scp/auth"
	"github.com/eugenmayer/go-sshclient/sshwrapper"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3Manager"
)

var client *github.Client
var ctx = context.Background()

func checkRepo(repo string) error {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: Cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	_, _, err := client.Repositories.ListBranches(ctx, Cfg.GithubOrg, repo, nil)
	if err != nil {
		LogError("No branch found. err=" + err.Error())
		return fmt.Errorf("Looks like this Repository is not part of the org or does not exist. Repo: %s", repo)

	}

	return nil
}

func createTag(tag, repo string) error {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: Cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	refs, _, err := client.Git.ListRefs(ctx, Cfg.GithubOrg, repo, nil)
	if err != nil {
		return err
	}
	checkTag := fmt.Sprintf("refs/tags/%s", tag)
	for _, ref := range refs {
		if ref.GetRef() == checkTag {
			return fmt.Errorf("Tag %s already exist. Please use another one", tag)
		}
	}

	ref, _, err := client.Git.GetRef(ctx, Cfg.GithubOrg, repo, "heads/master")
	if err != nil {
		return err
	}
	tagMessage := fmt.Sprintf("Tag %s", tag)
	tags := &github.Tag{
		Tag:     github.String(tag),
		Message: github.String(tagMessage),
		Object:  ref.Object,
	}

	_, _, err = client.Git.CreateTag(ctx, Cfg.GithubOrg, repo, tags)
	if err != nil {
		return err
	}

	tagRef := fmt.Sprintf("tags/%s", tag)
	refTag := &github.Reference{
		Ref:    github.String(tagRef),
		Object: ref.Object,
	}
	_, _, err = client.Git.CreateRef(ctx, Cfg.GithubOrg, repo, refTag)
	if err != nil {
		return err
	}

	return nil
}

func getReleaseArtifacts(tag, repo string) error {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: Cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	wait := 2400
	ctxRelease, cancel := context.WithTimeout(context.Background(), time.Duration(wait)*time.Second)
	defer cancel()

	assetID, releaseID, err := checkReleaseArtifact(ctxRelease, client, repo, tag)
	if err != nil {
		return err
	}

	assetURL, err := getAssetDownloadURL(ctx, client, repo, assetID)
	if err != nil {
		return err
	}

	filepath, err := downloadAsset(assetURL)
	if err != nil {
		return err
	}

	fileServerPath, err := copyFileToSigningServer(filepath)
	if err != nil {
		return err
	}

	err = signAsset(fileServerPath)
	if err != nil {
		return err
	}

	filename := strings.Split(filepath, "/")
	baseFilename := filename[len(filename)-1]
	localSignedFiles, err := copySignedFile(baseFilename)
	if err != nil {
		return err
	}

	err = uploadSignedArtifcatsToGithub(ctx, client, repo, releaseID, localSignedFiles)
	if err != nil {
		return err
	}

	err = uploadSignedArtifcatsToS3(localSignedFiles)
	if err != nil {
		return err
	}

	return nil
}

func checkReleaseArtifact(ctx context.Context, githubClient *github.Client, repo, tag string) (releaseAssetID, releaseID int64, err error) {
	LogInfo("Checking the if the release asset is available")
	for {
		repoReleases, _, err := client.Repositories.ListReleases(ctx, Cfg.GithubOrg, repo, nil)
		if err != nil {
			return -1, -1, err
		}
		for _, release := range repoReleases {
			if release.GetTagName() == tag {
				if release.GetAssetsURL() != "" && len(release.Assets) != 0 {
					return release.Assets[0].GetID(), release.GetID(), nil
				}
				LogInfo("Release found but no assets yet. Still waiting...")
			}
			LogInfo("waiting...")
		}

		select {
		case <-ctx.Done():
			return -1, -1, errors.New("timed out waiting for ok response")
		case <-time.After(30 * time.Second):
		}
	}
}

func getAssetDownloadURL(ctx context.Context, githubClient *github.Client, repo string, assetID int64) (string, error) {
	LogInfo("Getting the asset download URL")
	releaseAsset, _, err := client.Repositories.GetReleaseAsset(ctx, Cfg.GithubOrg, repo, assetID)
	if err != nil {
		return "", err
	}

	return releaseAsset.GetBrowserDownloadURL(), nil
}

func uploadSignedArtifcatsToGithub(ctx context.Context, githubClient *github.Client, repo string, releaseID int64, fileToUpload []string) error {
	LogInfo("Uploading signed assets to Github release")

	for _, file := range fileToUpload {
		opts := &github.UploadOptions{
			Name: file,
		}

		filePath := fmt.Sprintf("/tmp/%s", file)
		fileHandler, err := os.Open(filePath)
		defer fileHandler.Close()
		if err != nil {
			LogError("Error opening the file. err=" + err.Error())
			return err
		}
		_, _, err = client.Repositories.UploadReleaseAsset(ctx, Cfg.GithubOrg, repo, releaseID, opts, fileHandler)
		if err != nil {
			LogError("Error while uploading to github. err=" + err.Error())
			return err
		}
	}

	LogInfo("Done upload to Github")
	return nil
}

func downloadAsset(assetURL string) (string, error) {
	LogInfo("Will download the github release asset")
	url := strings.Split(assetURL, "/")
	filename := url[len(url)-1]

	resp, err := http.Get(assetURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Create the file
	filepath := fmt.Sprintf("/tmp/%s", filename)
	out, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}
	LogInfo("Done downloading")
	return filepath, nil
}

func copyFileToSigningServer(fileToCopy string) (string, error) {
	LogInfo("Will copy the artifact to the signing server")
	clientConfig, _ := auth.PrivateKey(Cfg.SSHUser, Cfg.SSHKeyPath, ssh.InsecureIgnoreHostKey())
	host := fmt.Sprintf("%s:22", Cfg.SSHHost)

	clientConfig.Timeout = 30 * time.Minute
	client := scp.NewClientWithTimeout(host, &clientConfig, 30*time.Minute)

	err := client.Connect()
	if err != nil {
		LogError("Couldn't establish a connection to the remote server. err=" + err.Error())
		return "", err
	}

	f, _ := os.Open(fileToCopy)
	defer client.Close()
	defer f.Close()

	filename := strings.Split(fileToCopy, "/")
	serverPath := fmt.Sprintf("/tmp/%s", filename[len(filename)-1])
	LogInfo(serverPath)
	err = client.CopyFile(f, serverPath, "0777")

	if err != nil {
		LogError("Error while copying file. err=" + err.Error())
		return "", err
	}

	LogInfo("Done copying")
	return serverPath, nil
}

func copySignedFile(baseFilename string) ([]string, error) {
	LogInfo("Will copy the signed file to upload to github")
	clientConfig, _ := auth.PrivateKey(Cfg.SSHUser, Cfg.SSHKeyPath, ssh.InsecureIgnoreHostKey())
	clientConfig.Timeout = 30 * time.Minute

	host := fmt.Sprintf("%s:22", Cfg.SSHHost)
	client := scp.NewClientWithTimeout(host, &clientConfig, 30*time.Minute)

	var filesToCopy []string
	filesToCopy = append(filesToCopy, fmt.Sprintf("%s.sig", baseFilename))
	filesToCopy = append(filesToCopy, fmt.Sprintf("%s.asc", baseFilename))

	for _, file := range filesToCopy {
		err := client.Connect()
		defer client.Close()
		if err != nil {
			LogError("Couldn't establish a connection to the remote server. err=" + err.Error())
			return []string{}, err
		}

		LogInfo(fmt.Sprintf("Will try to copy the remote file %s", file))
		remoteFile := fmt.Sprintf("/opt/plugin-signer/output/%s", file)
		fileHandler, _, err := client.CopyFromRemote(remoteFile)
		if err != nil {
			LogError("Error while copying the remote file. err=" + err.Error())
			return []string{}, err
		}

		saveFile := fmt.Sprintf("/tmp/%s", file)
		fo, err := os.Create(saveFile)
		defer fo.Close()
		if err != nil {
			LogError("Error while creating the local file. err=" + err.Error())
			return []string{}, err
		}

		w := bufio.NewWriter(fo)
		buf := make([]byte, 1024)
		for {
			// read a chunk
			n, errFile := fileHandler.Read(buf)
			if errFile != nil && errFile != io.EOF {
				return []string{}, errFile
			}
			if n == 0 {
				break
			}

			if _, errWrite := w.Write(buf[:n]); errWrite != nil {
				LogError("Error saving the signed file. err=" + errWrite.Error())
				return []string{}, errWrite
			}
		}

		if errFlush := w.Flush(); errFlush != nil {
			LogError("Error flushing file. err=" + errFlush.Error())
			return []string{}, errFlush
		}

	}

	LogInfo("Done copying signed file")
	return filesToCopy, nil
}

func signAsset(filePath string) error {
	LogInfo("Will sign the artifact")
	sshClient, err := sshwrapper.DefaultSshApiSetup(Cfg.SSHHost, 22, Cfg.SSHUser, Cfg.SSHKeyPath)
	if err != nil {
		LogError("Error whike setup the ssh connection. err=" + err.Error())
		return err
	}

	cmd := fmt.Sprintf("sudo -u signer /opt/plugin-signer/sign_plugin.sh %s", filePath)
	stdout, stderr, err := sshClient.Run(cmd)
	if err != nil {
		LogInfo(stdout)
		LogInfo(stderr)
		LogInfo(err.Error())
		return err
	}
	cmd = fmt.Sprintf("rm -f %s", filePath)
	stdout, stderr, err = sshClient.Run(cmd)
	if err != nil {
		LogInfo(stdout)
		LogInfo(stderr)
		LogInfo(err.Error())
		return err
	}
	LogInfo(stdout)
	LogInfo("Done signing the artifact")
	return nil
}

func uploadSignedArtifcatsToS3(fileToUpload []string) error {
	LogInfo("Uploading signed assets to S3")

	creds := credentials.NewStaticCredentials(Cfg.AWSAccessKey, Cfg.AWSSecretKey, "")
	_, err := creds.Get()
	if err != nil {
		return err
	}

	cfg := aws.NewConfig().WithRegion(Cfg.AWSRegion).WithCredentials(creds)
	sess := session.Must(session.NewSession(cfg))

	for _, fileToCopy := range fileToUpload {
		uploader := s3manager.NewUploader(sess)
		filePath := fmt.Sprintf("/tmp/%s", fileToCopy)
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %q, %v", filePath, err)
		}
		defer f.Close()

		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(Cfg.AWSS3PluginBucket),
			Key:    aws.String("release/" + fileToCopy),
			Body:   f,
		})
		if err != nil {
			return fmt.Errorf("failed to upload file, %v", err)
		}
		LogInfo("File uploaded to, %s\n", result.Location)
	}

	LogInfo("Done S3 upload")
	return nil
}
