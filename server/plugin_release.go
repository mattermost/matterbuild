// Copyright (c) 2018-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/eugenmayer/go-sshclient/sshwrapper"
	"github.com/google/go-github/github"
	"github.com/mattermost/matterbuild/utils"
	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/ssh"
)

var ErrTagExists = errors.New("tag already exists.")

// cutPlugin entry point to cutting a release for a plugin.
// This method DOES NOT generate github plugin release asset (<plugin>.tar.gz).
// It assumes the plugin release asset to be available on the repository's release.
// This generates:
// 1. Plugin signature (uploaded to github)
// 2. Platform specific plugin tars and their signatures (uploaded to s3 release bucket)
func cutPlugin(ctx context.Context, cfg *MatterbuildConfig, client *GithubClient, owner, repositoryName, tag string) error {
	pluginAsset, err := getPluginAsset(ctx, client, owner, repositoryName, tag)
	if err != nil {
		return errors.Wrap(err, "failed to get plugin asset")
	}

	// Download plugin tar into temp folder
	tmpFolder, err := ioutil.TempDir("", pluginAsset.GetName())
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tmpFolder)

	githubPluginFilePath, err := downloadAsset(ctx, client, owner, repositoryName, pluginAsset, tmpFolder)
	if err != nil {
		return errors.Wrap(err, "failed to download asset")
	}

	// Split plugin into platform specific tars
	platformPluginFilePaths, err := createPlatformPlugins(repositoryName, tag, githubPluginFilePath, tmpFolder)
	if err != nil {
		return errors.Wrap(err, "failed to create platform tars")
	}

	// Sign plugin tars and put them in tmpFolder. Signature files are assumed to be <path>.sig
	err = signPlugins(Cfg, append(platformPluginFilePaths, githubPluginFilePath), tmpFolder)
	if err != nil {
		return errors.Wrap(err, "failed to sign plugin tars")
	}

	// Upload github plugin tar signature to github
	githubPluginSignatureFilePath := githubPluginFilePath + ".sig"
	if err := uploadFilesToGithub(ctx, client, owner, repositoryName, tag, []string{githubPluginSignatureFilePath}); err != nil {
		return errors.Wrap(err, "failed to upload files to github")
	}

	// Duplicate github plugin tar and its signature that follows s3 release bucket naming convention
	s3PluginFilepath := filepath.Join(tmpFolder, fmt.Sprintf("%v-%v.tar.gz", repositoryName, tag))
	if err := os.Symlink(githubPluginFilePath, s3PluginFilepath); err != nil {
		return errors.Wrap(err, "failed to duplicate plugin file")
	}

	s3PluginSignatureFilepath := s3PluginFilepath + ".sig"
	if err := os.Symlink(githubPluginSignatureFilePath, s3PluginSignatureFilepath); err != nil {
		return errors.Wrap(err, "failed to duplicate signature file")
	}

	s3Bucket := []string{s3PluginFilepath, s3PluginSignatureFilepath}
	for _, p := range platformPluginFilePaths {
		s3Bucket = append(s3Bucket, p)
		s3Bucket = append(s3Bucket, fmt.Sprintf("%s.sig", p))
	}

	// Upload plugins and signatures to s3 release bucket
	if err := uploadToS3(ctx, Cfg, s3Bucket); err != nil {
		return errors.Wrap(err, "failed to upload to s3")
	}

	return nil
}

func checkRepo(ctx context.Context, client *GithubClient, owner, repo string) error {
	result, _, err := client.Search.Repositories(ctx, fmt.Sprintf("repo:%s/%s", owner, repo), nil)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch github repo %s", repo)
	}

	if result.GetTotal() == 0 {
		return errors.Errorf("looks like this repository is not part of the org or does not exist. Repo: %s", repo)
	}

	return nil
}

func getReleaseByTag(ctx context.Context, client *GithubClient, owner, repositoryName, tag string) (*github.RepositoryRelease, error) {
	release, _, err := client.Repositories.GetReleaseByTag(ctx, owner, repositoryName, tag)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get release by tag")
	}

	return release, nil
}

// createTag creates a new tag at the given commit for the repository.
// Returns ErrTagExists if tag already exists, nil if successful and an error otherwise.
func createTag(ctx context.Context, client *GithubClient, owner, repository, tag, commitSHA string) error {
	tagRef := fmt.Sprintf("tags/%s", tag)
	refs, _, err := client.Git.GetRefs(ctx, owner, repository, tagRef)
	if err != nil {
		var gerr *github.ErrorResponse
		if errors.As(err, &gerr) && gerr.Response.StatusCode == http.StatusNotFound {
			LogInfo("tag %s was not found, creating tag", tag)
		} else {
			return errors.Wrapf(err, "failed to get github tag")
		}
	}
	for _, ref := range refs {
		if strings.HasSuffix(ref.GetRef(), tagRef) {
			return ErrTagExists
		}
	}

	if commitSHA == "" {
		// Use master's tip if commitSHA is not provided
		var ref *github.Reference
		ref, _, err = client.Git.GetRef(ctx, owner, repository, "heads/master")
		if err != nil {
			return errors.Wrap(err, "failed to get github ref")
		}

		commitSHA = *ref.Object.SHA
	} else {
		// Check if sha exists
		_, _, err = client.Repositories.GetCommit(ctx, owner, repository, commitSHA)
		if err != nil {
			return errors.Wrap(err, "failed to fetch sha details")
		}
	}

	tagObject := &github.GitObject{
		SHA:  github.String(commitSHA),
		Type: github.String("commit"),
	}

	githubTag := &github.Tag{
		Tag:     github.String(tag),
		Message: github.String(tag),
		Object:  tagObject,
	}

	if _, _, err = client.Git.CreateTag(ctx, owner, repository, githubTag); err != nil {
		return errors.Wrap(err, "failed to create tag")
	}

	refTag := &github.Reference{
		Ref:    github.String(fmt.Sprintf("tags/%s", tag)),
		Object: tagObject,
	}

	if _, _, err = client.Git.CreateRef(ctx, owner, repository, refTag); err != nil {
		return errors.Wrap(err, "failed to create ref")
	}

	return nil
}

// signPlugins signs plugin tar files and saves them in the tmpFolder.
// Signature files are named <filePath>.sig.
func signPlugins(cfg *MatterbuildConfig, filePaths []string, tmpFolder string) error {
	// Copy files to remote server.
	remotePaths, err := copyFilesToRemoteServer(cfg, filePaths)
	if err != nil {
		return errors.Wrap(err, "error while copying files")
	}

	// Sign files on remote server.
	remoteSignaturePaths, err := signFilesOnRemoteServer(cfg, remotePaths)
	if err != nil {
		return errors.Wrap(err, "error while signing files")
	}

	// Fetch signatures from remote server.
	if err := copyFilesFromRemoteServer(cfg, remoteSignaturePaths, tmpFolder); err != nil {
		return errors.Wrap(err, "error while copying remote files")
	}

	// Verify signatures.
	if err := verifySignatures(filePaths); err != nil {
		return errors.Wrap(err, "failed signature verification")
	}

	// All is well, remove *.tar.gz files from remote server.
	if err := removeFilesFromRemoteServer(cfg, remotePaths); err != nil {
		return errors.Wrap(err, "failed to remove files from remote server")
	}

	return nil
}

// copyFilesFromRemoteServer copies remoteFiles to pluginFolder.
func copyFilesFromRemoteServer(cfg *MatterbuildConfig, remoteFiles []string, pluginFolder string) error {
	LogInfo("Copying files from remote server")

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to get sftp client")
	}
	defer sftp.Close()

	for _, remoteFile := range remoteFiles {
		srcFile, err := sftp.Open(remoteFile)
		if err != nil {
			return errors.Wrapf(err, "failed to open remote file %s,", remoteFile)
		}
		defer srcFile.Close()

		destination := filepath.Join(pluginFolder, filepath.Base(remoteFile))
		LogInfo("copying %s -> %s", remoteFile, destination)
		dstFile, err := os.Create(destination)
		if err != nil {
			return errors.Wrapf(err, "failed to create file %s,", destination)
		}
		defer dstFile.Close()

		if _, err := srcFile.WriteTo(dstFile); err != nil {
			return errors.Wrap(err, "error while reading from remote buffer")
		}
	}

	LogInfo("Done copying files from remote server")
	return nil
}

func copyFilesToRemoteServer(cfg *MatterbuildConfig, filePaths []string) ([]string, error) {
	LogInfo("Copying files to the signing server")
	var result []string

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sftp client")
	}
	defer sftp.Close()

	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open file %s,", filePath)
		}
		defer f.Close()

		serverPath := filepath.Join("/tmp", filepath.Base(filePath))
		LogInfo("copying %s -> %s", filePath, serverPath)

		// Open the source file
		srcFile, err := sftp.Create(serverPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create remote file %s,", serverPath)
		}
		defer srcFile.Close()

		if _, err := srcFile.ReadFrom(f); err != nil {
			return nil, errors.Wrap(err, "failed to read from file")
		}

		result = append(result, serverPath)
	}

	LogInfo("Done copying")
	return result, nil
}

func removeFilesFromRemoteServer(cfg *MatterbuildConfig, remoteFiles []string) error {
	LogInfo("Removing files from remote server")

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to get sftp client")
	}
	defer sftp.Close()

	for _, remoteFile := range remoteFiles {
		if err := sftp.Remove(remoteFile); err != nil {
			return errors.Wrapf(err, "failed to remove %s,", remoteFile)
		}
	}

	LogInfo("Done copying files from remote server")
	return nil
}

// signFilesOnRemoteServer signs and removes files from the remote server.
// Returns signature filepaths.
func signFilesOnRemoteServer(cfg *MatterbuildConfig, remoteFilePaths []string) ([]string, error) {
	LogInfo("Starting to sign %s", remoteFilePaths)
	var result []string

	clientConfig, err := getSSHClientConfig(cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath, cfg.PluginSigningSSHPublicCertPath, cfg.PluginSigningSSHHostPublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup client config")
	}
	sshClient := sshwrapper.NewSshApi(cfg.PluginSigningSSHHost, 22, cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath)
	sshClient.SshConfig = clientConfig

	for _, remoteFilePath := range remoteFilePaths {
		LogInfo("Signing " + remoteFilePath)

		stdout, stderr, err := sshClient.Run(fmt.Sprintf("sudo -u signer /opt/plugin-signer/sign_plugin.sh %s", remoteFilePath))
		LogInfo(stdout)
		LogInfo(stderr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to run signer script")
		}

		result = append(result, fmt.Sprintf("/opt/plugin-signer/output/%s.sig", filepath.Base(remoteFilePath)))
	}

	LogInfo("Done signing")
	return result, nil
}

// verifySignatures verifies plugin files, assumes signatures are <filepath>.sig.
func verifySignatures(pluginFilePaths []string) error {
	block, err := armor.Decode(bytes.NewReader(mattermostPluginPublicKey))
	if err != nil {
		return errors.Wrap(err, "failed to decode public key")
	}

	keyring, err := openpgp.ReadKeyRing(block.Body)
	if err != nil {
		return errors.Wrap(err, "can't read public key")
	}

	for _, pluginFilePath := range pluginFilePaths {
		signedFile, err := os.Open(pluginFilePath)
		if err != nil {
			return errors.Wrap(err, "cannot read signed file")
		}
		defer signedFile.Close()

		// Assume signature is always <filepath>.sig
		signatureFile, err := os.Open(fmt.Sprintf("%s.sig", pluginFilePath))
		if err != nil {
			return errors.Wrap(err, "cannot read signature file")
		}
		defer signatureFile.Close()

		if _, err = openpgp.CheckDetachedSignature(keyring, signedFile, signatureFile); err != nil {
			return errors.Wrap(err, "error while checking the signature")
		}
	}

	LogInfo("Signatures verified for %+v", pluginFilePaths)
	return nil
}

// createPlatformPlugins splits plugin tar into platform specific plugin tars.
// Returns paths to platform plugin tars if successful, or an error otherwise.
func createPlatformPlugins(repositoryName, tag, pluginFilePath, pluginFolder string) ([]string, error) {
	var result []string
	if err := hasAllPlatformBinaries(pluginFilePath); err != nil {
		return nil, errors.Wrap(err, "plugin tar missing platform binaries")
	}

	platformFileExclusion := map[string][]string{
		"osx-amd64":     {"windows", "linux"},
		"windows-amd64": {"darwin", "linux"},
		"linux-amd64":   {"darwin", "windows"},
	}

	platformFileInclusion := map[string]string{
		"osx-amd64":     "plugin-darwin-amd64",
		"windows-amd64": "plugin-windows-amd64.exe",
		"linux-amd64":   "plugin-linux-amd64",
	}

	for platform, excludeOtherPlatforms := range platformFileExclusion {
		platformTarPath := filepath.Join(pluginFolder, fmt.Sprintf("%v-%v-%v.tar.gz", repositoryName, tag, platform))
		err := createPlatformPlugin(pluginFilePath, excludeOtherPlatforms, platformTarPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create platform tar for %s", platformTarPath)
		}

		// Verify if this tar contains the correct platform binary
		found, err := archiveContains(platformTarPath, "plugin-")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check files in archive %s,", platformTarPath)
		}
		if len(found) != 1 || found[0] != platformFileInclusion[platform] {
			return nil, errors.Errorf("found wrong platform binary in %s, expected %s, but found %v", platformTarPath, platformFileInclusion[platform], found)
		}

		result = append(result, platformTarPath)
	}

	return result, nil
}

func createPlatformPlugin(pluginFilePath string, excludePlatforms []string, platformTarPath string) error {
	// Couldn't achieve gzip level compressions with golang archive api, using shell cmds instead.
	tarCmdStr := "tar"
	if runtime.GOOS == "darwin" {
		tarCmdStr = "gtar"
	}

	deleteWildcards := []string{"--wildcards"}
	for _, deleteWildcard := range excludePlatforms {
		deleteWildcards = append(deleteWildcards, "--delete")
		deleteWildcards = append(deleteWildcards, fmt.Sprintf("*%s*", deleteWildcard))
	}

	f, err := os.Create(platformTarPath)
	if err != nil {
		return errors.Wrapf(err, "failed to create platform tar file %s", platformTarPath)
	}

	catCmd := exec.Command("cat", pluginFilePath)
	gunzipCmd := exec.Command("gunzip")
	tarCmd := exec.Command(tarCmdStr, deleteWildcards...)
	gzipCmd := exec.Command("gzip")
	cmds := []*exec.Cmd{catCmd, gunzipCmd, tarCmd, gzipCmd}

	utils.AssemblePipes(cmds, os.Stdin, f)
	if err = utils.RunCmds(cmds); err != nil {
		return errors.Wrapf(err, "failed to run shell cmds")
	}

	return nil
}

// downloadAsset Downloads asset into a given folder and returns its path.
func downloadAsset(ctx context.Context, client *GithubClient, owner, repositoryName string, asset *github.ReleaseAsset, folder string) (filePath string, err error) {
	LogInfo("Downloading github release asset")

	rc, redirectURL, err := client.Repositories.DownloadReleaseAsset(ctx, owner, repositoryName, asset.GetID())
	if err != nil {
		return "", errors.Wrapf(err, "failed to fetch asset %s,", asset.GetName())
	}

	// Create local file
	pathToFile := filepath.Join(folder, asset.GetName())
	out, err := os.Create(pathToFile)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create file for asset %s,", pathToFile)
	}
	defer out.Close()

	if redirectURL != "" {
		var resp *http.Response
		resp, err = http.Get(redirectURL)
		if err != nil {
			return "", errors.Wrapf(err, "failed to fetch asset %s,", redirectURL)
		}
		defer resp.Body.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return "", errors.Wrapf(err, "failed to copy resp.Body")
		}

		return pathToFile, nil
	}

	if rc != nil {
		_, err = io.Copy(out, rc)
		if err != nil {
			return "", errors.Wrapf(err, "failed to copy rc")
		}

		return pathToFile, nil
	}

	return "", errors.Errorf("failed to download release asset %s", asset.GetName())
}

// getPluginAsset polls till it finds the plugin tar file.
func getPluginAsset(ctx context.Context, githubClient *GithubClient, owner, repo, tag string) (*github.ReleaseAsset, error) {
	LogInfo("Checking if the release asset is available")

	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	for {
		// Using timer to avoid memory leaks
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()

		release, _, err := githubClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
		if err != nil {
			var gerr *github.ErrorResponse
			if errors.As(err, &gerr) && gerr.Response.StatusCode == http.StatusNotFound {
				LogInfo("get release by tag %s was not found, trying again shortly", tag)
			} else {
				return nil, errors.Wrap(err, "failed to get release by tag")
			}
		}

		if release != nil {
			var foundPluginAsset *github.ReleaseAsset
			for i := range release.Assets {
				assetName := release.Assets[i].GetName()
				if strings.HasSuffix(assetName, ".tar.gz") {
					if foundPluginAsset != nil {
						return nil, errors.Errorf("found unexpected file %s", assetName)
					}
					foundPluginAsset = &release.Assets[i]
				}
			}

			if foundPluginAsset != nil {
				return foundPluginAsset, nil
			}
			LogInfo("Release found but no assets yet. Still waiting...")
		}

		select {
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for ok response")
		case <-timer.C:
		}
	}
}

func uploadFilesToGithub(ctx context.Context, githubClient *GithubClient, owner, repo, tag string, filePaths []string) error {
	LogInfo("Uploading files to github")

	release, _, err := githubClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return errors.Wrap(err, "failed to get release by tag")
	}

	for _, filePath := range filePaths {
		assetName := filepath.Base(filePath)
		opts := &github.UploadOptions{
			Name: assetName,
		}

		file, err := os.Open(filePath)
		if err != nil {
			return errors.Wrap(err, "failed to open file to upload")
		}
		defer file.Close()

		// Attempt to remove asset, incase it exists.
		asset, err := getReleaseAsset(ctx, owner, githubClient, repo, release.GetID(), assetName)
		if err == nil {
			if _, err = githubClient.Repositories.DeleteReleaseAsset(ctx, owner, repo, asset.GetID()); err != nil {
				return errors.Wrapf(err, "failed to remove asset (%s) from repo", assetName)
			}
			LogInfo("removed release asset (%s) for repo (%s), tag (%s)", assetName, repo, tag)
		} else {
			LogInfo("no existing release asset (%s) found, moving on to uploading it, err=%s", assetName, err.Error())
		}

		_, _, err = githubClient.Repositories.UploadReleaseAsset(ctx, owner, repo, release.GetID(), opts, file)
		if err != nil {
			return errors.Wrap(err, "error while uploading to github.")
		}
	}

	LogInfo("Done uploading to Github")
	return nil
}

func getReleaseAsset(ctx context.Context, owner string, githubClient *GithubClient, repositoryName string, releaseID int64, assetName string) (*github.ReleaseAsset, error) {
	assets, _, err := githubClient.Repositories.ListReleaseAssets(ctx, owner, repositoryName, releaseID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list release assets.")
	}

	for _, asset := range assets {
		if asset.GetName() == assetName {
			return asset, nil
		}
	}

	return nil, errors.Errorf("could not find github release asset %s", assetName)
}

func uploadToS3(ctx context.Context, cfg *MatterbuildConfig, filePaths []string) error {
	LogInfo("Uploading files to S3")

	creds := credentials.NewStaticCredentials(cfg.PluginSigningAWSAccessKey, cfg.PluginSigningAWSSecretKey, "")
	awsCfg := aws.NewConfig().WithRegion(cfg.PluginSigningAWSRegion).WithCredentials(creds)
	awsSession := session.Must(session.NewSession(awsCfg))

	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			return errors.Wrapf(err, "failed to open file %v", filePath)
		}
		defer f.Close()

		uploader := s3manager.NewUploader(awsSession)
		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(cfg.PluginSigningAWSS3PluginBucket),
			Key:    aws.String("release/" + filepath.Base(filePath)),
			Body:   f,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to upload file, %v", filePath)
		}
		LogInfo("File uploaded to, %s\n", result.Location)
	}

	LogInfo("Done S3 upload")
	return nil
}

func getPluginSigningSftpClient(cfg *MatterbuildConfig) (*sftp.Client, error) {
	clientConfig, err := getSSHClientConfig(cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath, cfg.PluginSigningSSHPublicCertPath, cfg.PluginSigningSSHHostPublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup client config")
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%v:22", cfg.PluginSigningSSHHost), clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup client config")
	}

	sftp, err := sftp.NewClient(client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup sftp client")
	}

	return sftp, nil
}

// hasAllPlatformBinaries verifies if plugin tar contains 3 platform binaries.
func hasAllPlatformBinaries(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to setup sftp client")
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrap(err, "failed to read gzip data")
	}

	tarReader := tar.NewReader(gzf)
	serverDist := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to read tar data")
		}

		name := header.Name
		switch header.Typeflag {
		case tar.TypeReg:
			if strings.Contains(name, "plugin-linux-amd64") || strings.Contains(name, "plugin-windows-amd64.exe") || strings.Contains(name, "plugin-darwin-amd64") {
				serverDist[name] = struct{}{}
			}
		}
	}

	if len(serverDist) != 3 {
		return errors.Errorf("plugin tar contains %+v, but should contain all platform binaries", serverDist)
	}

	return nil
}

// archiveContains returns filenames that matches a given string.
func archiveContains(filePath string, contains string) ([]string, error) {
	var result []string
	f, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open archive %s,", filePath)
	}

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}

	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read next %s,", filePath)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			baseName := filepath.Base(header.Name)
			if strings.Contains(baseName, contains) {
				result = append(result, baseName)
			}
		}
	}

	return result, nil
}

// getSSHClientConfig Loads a private and public key from "path" and returns a SSH ClientConfig to authenticate with the server.
func getSSHClientConfig(username, path, certPath, hostPublicKey string) (*ssh.ClientConfig, error) {
	privateKey, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read key path")
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse private key")
	}

	// Load the certificate if present
	if certPath != "" {
		var cert []byte
		cert, err = ioutil.ReadFile(certPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read cert path")
		}

		var pk ssh.PublicKey
		pk, _, _, _, err = ssh.ParseAuthorizedKey(cert)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse authorized key")
		}

		signer, err = ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get cert signer")
		}
	}

	if hostPublicKey == "" {
		return nil, errors.New("missing host public key")
	}

	hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostPublicKey))
	if err != nil {
		return nil, errors.Wrap(err, "failed parse host public key")
	}

	return &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         30 * time.Second,
	}, nil
}

// getSuccessMessage return the plugin release success message to get posted into a channel.
// releaseURL and commitSHA may be empty.
func getSuccessMessage(tag, repo, commitSHA, releaseURL, username string) string {
	branch := fmt.Sprintf("add_%s_%s", repo, tag)

	const codeSeperator = "```"

	marketplaceCommand := "\n" + codeSeperator +
		fmt.Sprintf(`
git checkout production
git pull
git checkout -b %[3]s
go run ./cmd/generator/ add %[2]s %[1]s [--official|--community]
`, tag, repo, branch) +
		codeSeperator + "\n" +
		"Use `--official` for plugins maintained by Matttermost and `--community` for ones mainted by the Open Source community.\n" +
		"You might want to use other flag like `--beta` to add a `Beta` label.\n" +
		"\n" +
		"Then review your changes by running `git diff plugins.json`\n" +
		codeSeperator +
		fmt.Sprintf(`
make generate
git commit plugins.json data/statik/statik.go -m "Add %[1]s of %[2]s to the Marketplace"
git push --set-upstream origin %[3]s
git checkout master
`, tag, repo, branch) +
		codeSeperator + "\n"

	url := fmt.Sprintf(
		"https://github.com/mattermost/mattermost-marketplace/compare/production...%s?quick_pull=1&labels=3:+QA+Review,2:+Dev+Review",
		branch,
	)

	msg := fmt.Sprintf("@%s A Plugin was successfully signed and uploaded to Github and S3.\nTag: **%s**\nRepo: **%s**\n", username, tag, repo)

	if commitSHA != "" {
		msg += fmt.Sprintf("CommitSHA: **%s**\n", commitSHA)
	}

	if releaseURL != "" {
		msg += fmt.Sprintf("[Release Link](%s)\n", releaseURL)
	}

	msg += fmt.Sprintf(
		"To add this release to the Plugin Marketplace run inside your local Marketplace repository:%sUse %s to open a Pull Request.",
		marketplaceCommand, url,
	)

	return msg
}
