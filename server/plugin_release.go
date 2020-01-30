// Copyright (c) 2018-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
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

	"github.com/google/go-github/github"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"

	"github.com/eugenmayer/go-sshclient/sshwrapper"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

var ErrTagExists = fmt.Errorf("Tag already exists.")

// cutPlugin entry point to cutting a release for a plugin.
// This method DOES NOT generate github plugin release asset (<plugin>.tar.gz).
// It assumes the plugin release asset to be available on the repository's release.
// This generates:
// 1. Plugin signature (uploaded to github)
// 2. Platform specific plugin tars and their signatures (uploaded to s3 release bucket)
func cutPlugin(cfg *MatterbuildConfig, owner, repositoryName, tag string) error {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	pluginAsset, err := getPluginAsset(ctx, client, owner, repositoryName, tag)
	if err != nil {
		return err
	}

	// Download plugin tar into temp folder
	tmpFolder, err := ioutil.TempDir("", pluginAsset.GetName())
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpFolder)

	githubPluginFilePath, err := downloadAsset(ctx, pluginAsset, tmpFolder)
	if err != nil {
		return err
	}

	// Split plugin into platform specific tars
	platformPluginFilePaths, err := createPlatformPlugins(repositoryName, tag, githubPluginFilePath, tmpFolder)
	if err != nil {
		return err
	}

	// Sign plugin tars and put them in tmpFolder. Signature files are assumed to be <path>.sig
	err = signPlugins(Cfg, append(platformPluginFilePaths, githubPluginFilePath), tmpFolder)
	if err != nil {
		return err
	}

	// Upload github plugin tar signature to github
	githubPluginSignatureFilePath := fmt.Sprintf("%s.sig", githubPluginFilePath)
	if err := uploadFilesToGithub(ctx, client, owner, repositoryName, tag, []string{githubPluginSignatureFilePath}); err != nil {
		return err
	}

	// Duplicate github plugin tar signature that follows s3 release bucket naming convention
	s3PluginSignatureFilepath := filepath.Join(tmpFolder, fmt.Sprintf("%v-%v.tar.gz.sig", repositoryName, tag))
	if err := os.Symlink(githubPluginSignatureFilePath, s3PluginSignatureFilepath); err != nil {
		return fmt.Errorf("failed to duplicate signature file err=%w", err)
	}

	s3Bucket := []string{s3PluginSignatureFilepath}
	for _, p := range platformPluginFilePaths {
		s3Bucket = append(s3Bucket, p)
		s3Bucket = append(s3Bucket, fmt.Sprintf("%s.sig", p))
	}

	// Upload plugins and signatures to s3 release bucket
	if err := uploadToS3(ctx, Cfg, s3Bucket); err != nil {
		return fmt.Errorf("failed to upload to s3 err=%w", err)
	}

	return nil
}

// checkRepo checks if repo exists.
func checkRepo(cfg *MatterbuildConfig, owner, repo string) error {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	if _, _, err := client.Repositories.ListBranches(ctx, owner, repo, nil); err != nil {
		LogError("No branch found. err=" + err.Error())
		return fmt.Errorf("Looks like this Repository is not part of the org or does not exist. Repo: %s", repo)
	}

	return nil
}

// getReleaseByTag gets github release by tag.
func getReleaseByTag(cfg *MatterbuildConfig, owner, repositoryName, tag string) (*github.RepositoryRelease, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	release, _, err := client.Repositories.GetReleaseByTag(ctx, owner, repositoryName, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to get release by tag err=%w", err)
	}

	return release, nil
}

// createTag creates a new tag at master for repo.
// Returns ErrTagExists if tag already exists, nil if successful and an error otherwise.
func createTag(cfg *MatterbuildConfig, owner, tag, repository string) error {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GithubAccessToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	refs, _, err := client.Git.GetRefs(ctx, owner, repository, fmt.Sprintf("tags/%s", tag))
	if err != nil {
		if err, ok := err.(*github.ErrorResponse); ok && err.Response.StatusCode == http.StatusNotFound {
			LogInfo("tag %s was not found, moving on to creating tag", tag)
		} else {
			return err
		}
	} else if len(refs) > 0 {
		return ErrTagExists
	}

	ref, _, err := client.Git.GetRef(ctx, owner, repository, "heads/master")
	if err != nil {
		return err
	}

	tags := &github.Tag{
		Tag:     github.String(tag),
		Message: github.String(fmt.Sprintf("Tag %s", tag)),
		Object:  ref.Object,
	}

	if _, _, err = client.Git.CreateTag(ctx, owner, repository, tags); err != nil {
		return err
	}

	refTag := &github.Reference{
		Ref:    github.String(fmt.Sprintf("tags/%s", tag)),
		Object: ref.Object,
	}

	if _, _, err = client.Git.CreateRef(ctx, owner, repository, refTag); err != nil {
		return err
	}

	return nil
}

// signPlugins signs plugin tar files and saves them in the tmpFolder.
// Signature files are named <filePath>.sig.
func signPlugins(cfg *MatterbuildConfig, filePaths []string, tmpFolder string) error {
	// Copy files to remote server.
	remotePaths, err := copyFilesToRemoteServer(cfg, filePaths)
	if err != nil {
		return fmt.Errorf("error while copying files err=%w", err)
	}

	// Sign files on remote server.
	remoteSignaturePaths, err := signFilesOnRemoteServer(cfg, remotePaths)
	if err != nil {
		return fmt.Errorf("error while signing files err=%w", err)
	}

	// Fetch signatures from remote server.
	if err := copyFilesFromRemoteServer(cfg, remoteSignaturePaths, tmpFolder); err != nil {
		return fmt.Errorf("error while copying remote files err=%w", err)
	}

	// Verify signatures.
	if err := verifySignatures(filePaths); err != nil {
		return fmt.Errorf("failed signature verification err=%w", err)
	}

	// All is well, remove *.tar.gz files from remote server.
	if err := removeFilesFromRemoteServer(cfg, remotePaths); err != nil {
		return fmt.Errorf("failed to remove files from remote server err=%w", err)
	}

	return nil
}

// copyFilesFromRemoteServer copies remoteFiles to pluginFolder.
func copyFilesFromRemoteServer(cfg *MatterbuildConfig, remoteFiles []string, pluginFolder string) error {
	LogInfo("Copying files from remote server")

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return err
	}
	defer sftp.Close()

	for _, remoteFile := range remoteFiles {
		srcFile, err := sftp.Open(remoteFile)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destination := filepath.Join(pluginFolder, filepath.Base(remoteFile))
		LogInfo("%s -> %s", remoteFile, destination)
		dstFile, err := os.Create(destination)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		if _, err := srcFile.WriteTo(dstFile); err != nil {
			return fmt.Errorf("error while reading from remote buffer err=%w", err)
		}
	}

	LogInfo("Done copying files from remote server")
	return nil
}

// copyFilesToRemoteServer copies files to the remote server.
func copyFilesToRemoteServer(cfg *MatterbuildConfig, filePaths []string) ([]string, error) {
	LogInfo("Copying files to the signing server")
	var result []string

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return nil, err
	}
	defer sftp.Close()

	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		serverPath := filepath.Join("/tmp", filepath.Base(filePath))
		LogInfo("%s -> %s", filePath, serverPath)

		// Open the source file
		srcFile, err := sftp.Create(serverPath)
		if err != nil {
			return nil, err
		}
		defer srcFile.Close()

		if _, err := srcFile.ReadFrom(f); err != nil {
			return nil, err
		}

		result = append(result, serverPath)
	}

	LogInfo("Done copying")
	return result, nil
}

// removeFilesFromRemoteServer removes files from the remote server..
func removeFilesFromRemoteServer(cfg *MatterbuildConfig, remoteFiles []string) error {
	LogInfo("Removing files from remote server")

	sftp, err := getPluginSigningSftpClient(cfg)
	if err != nil {
		return err
	}
	defer sftp.Close()

	for _, remoteFile := range remoteFiles {
		if err := sftp.Remove(remoteFile); err != nil {
			return fmt.Errorf("failed to remove %s, err=%w", remoteFile, err)
		}
	}

	LogInfo("Done copying files from remote server")
	return nil
}

// signFilesOnRemoteServer signs and removes files from the remote server. Returns signature filepaths
func signFilesOnRemoteServer(cfg *MatterbuildConfig, remoteFilePaths []string) ([]string, error) {
	LogInfo("Starting to sign %s", remoteFilePaths)
	var result []string

	clientConfig, err := privateKey(cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath, cfg.PluginSigningSSHPublicCertPath, ssh.InsecureIgnoreHostKey())
	if err != nil {
		return nil, fmt.Errorf("failed to setup client config err=%w", err)
	}
	sshClient := sshwrapper.NewSshApi(cfg.PluginSigningSSHHost, 22, cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath)
	sshClient.SshConfig = &clientConfig

	for _, remoteFilePath := range remoteFilePaths {
		LogInfo("Signing " + remoteFilePath)

		stdout, stderr, err := sshClient.Run(fmt.Sprintf("sudo -u signer /opt/plugin-signer/sign_plugin.sh %s", remoteFilePath))
		LogInfo(stdout)
		LogInfo(stderr)
		if err != nil {
			return nil, err
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
		return fmt.Errorf("failed to decode public key err=%w", err)
	}

	keyring, err := openpgp.ReadKeyRing(block.Body)
	if err != nil {
		return fmt.Errorf("can't read public key err=%w", err)
	}

	for _, pluginFilePath := range pluginFilePaths {
		signedFile, err := os.Open(pluginFilePath)
		if err != nil {
			return fmt.Errorf("cannot read signed file err=%w", err)
		}
		defer signedFile.Close()

		// Assume signature is always <filepath>.sig
		signatureFile, err := os.Open(fmt.Sprintf("%s.sig", pluginFilePath))
		if err != nil {
			return fmt.Errorf("cannot read signature file err=%w", err)
		}
		defer signatureFile.Close()

		if _, err = openpgp.CheckDetachedSignature(keyring, signedFile, signatureFile); err != nil {
			return fmt.Errorf("error while checking the signature err=%w", err)
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
		return nil, err
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
		deleteWildcards := ""
		for _, deleteWildcard := range excludeOtherPlatforms {
			deleteWildcards += fmt.Sprintf(`--delete "*%s*" `, deleteWildcard)
		}

		// Couldn't achieve gzip level compressions with golang archive api, using shell cmds instead.
		platformTar := filepath.Join(pluginFolder, fmt.Sprintf("%v-%v-%v.tar.gz", repositoryName, tag, platform))
		tarCmd := "tar"
		if runtime.GOOS == "darwin" {
			tarCmd = "gtar"
		}
		cmd := fmt.Sprintf(`cat %s | gunzip | %s --wildcards %s | gzip > %s`, pluginFilePath, tarCmd, deleteWildcards, platformTar)
		if _, err := exec.Command("bash", "-c", cmd).Output(); err != nil {
			return nil, err
		}

		// Verify if this tar contains the correct platform binary
		found, err := archiveContains(platformTar, "plugin-")
		if err != nil {
			return nil, err
		}
		if len(found) != 1 || found[0] != platformFileInclusion[platform] {
			return nil, fmt.Errorf("found wrong platform binary in %s, expected %s, but found %v", platformTar, platformFileInclusion[platform], found)
		}

		result = append(result, platformTar)
	}

	return result, nil
}

func downloadAsset(ctx context.Context, asset *github.ReleaseAsset, folder string) (filePath string, err error) {
	LogInfo("Downloading the github release asset")

	// Get the data
	resp, err := http.Get(asset.GetBrowserDownloadURL())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Create the file
	pathToFile := filepath.Join(folder, asset.GetName())
	out, err := os.Create(pathToFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", nil
	}

	return pathToFile, nil
}

// getPluginAsset polls till it finds the plugin tar file.
func getPluginAsset(ctx context.Context, githubClient *github.Client, owner, repo, tag string) (*github.ReleaseAsset, error) {
	LogInfo("Checking if the release asset is available")

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	for {
		// Using timer to avoid memory leaks
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()

		release, _, err := githubClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
		if err != nil {
			if err, ok := err.(*github.ErrorResponse); ok && err.Response.StatusCode == http.StatusNotFound {
				LogInfo("get release by tag %s was not found, trying again shortly", tag)
			} else {
				return nil, err
			}
		}

		if release != nil {
			var pluginAsset *github.ReleaseAsset
			for i := range release.Assets {
				assetName := release.Assets[i].GetName()
				if strings.HasSuffix(assetName, ".tar.gz") {
					if pluginAsset != nil {
						return nil, fmt.Errorf("found unexpected file %s", assetName)
					}
					pluginAsset = &release.Assets[i]
				}
			}

			if pluginAsset != nil {
				return pluginAsset, nil
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

func uploadFilesToGithub(ctx context.Context, githubClient *github.Client, owner, repo, tag string, filePaths []string) error {
	LogInfo("Uploading files to github")

	release, _, err := githubClient.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return fmt.Errorf("failed to get release by tag err=%w", err)
	}

	for _, filePath := range filePaths {
		assetName := filepath.Base(filePath)
		opts := &github.UploadOptions{
			Name: assetName,
		}

		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file to upload err=%w", err)
		}
		defer file.Close()

		// Attempt to remove asset, incase it exists.
		asset, err := getReleaseAsset(ctx, owner, githubClient, repo, release.GetID(), assetName)
		if err != nil {
			LogInfo("no existing release asset (%s) found, moving on to uploading it, err=%s", assetName, err.Error())
		} else {
			if _, err = githubClient.Repositories.DeleteReleaseAsset(ctx, owner, repo, asset.GetID()); err != nil {
				return fmt.Errorf("failed to remove asset (%s) from repo err=%w", assetName, err)
			}
			LogInfo("removed release asset (%s) for repo (%s), tag (%s)", assetName, repo, tag)
		}

		_, _, err = githubClient.Repositories.UploadReleaseAsset(ctx, owner, repo, release.GetID(), opts, file)
		if err != nil {
			return fmt.Errorf("error while uploading to github. err=%w", err)
		}
	}

	LogInfo("Done uploading to Github")
	return nil
}

func getReleaseAsset(ctx context.Context, owner string, githubClient *github.Client, repositoryName string, releaseID int64, assetName string) (*github.ReleaseAsset, error) {
	assets, _, err := githubClient.Repositories.ListReleaseAssets(ctx, owner, repositoryName, releaseID, nil)
	if err != nil {
		return nil, err
	}

	for _, asset := range assets {
		if asset.GetName() == assetName {
			return asset, nil
		}
	}

	return nil, fmt.Errorf("could not find github release asset %s", assetName)
}

func uploadToS3(ctx context.Context, cfg *MatterbuildConfig, filePaths []string) error {
	LogInfo("Uploading files to S3")

	creds := credentials.NewStaticCredentials(cfg.PluginSigningAWSAccessKey, cfg.PluginSigningAWSSecretKey, "")
	awsCfg := aws.NewConfig().WithRegion(cfg.PluginSigningAWSRegion).WithCredentials(creds)
	awsSession := session.Must(session.NewSession(awsCfg))

	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %q, %v", filePath, err)
		}
		defer f.Close()

		uploader := s3manager.NewUploader(awsSession)
		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(cfg.PluginSigningAWSS3PluginBucket),
			Key:    aws.String("release/" + filepath.Base(filePath)),
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

func getPluginSigningSftpClient(cfg *MatterbuildConfig) (*sftp.Client, error) {
	clientConfig, err := privateKey(cfg.PluginSigningSSHUser, cfg.PluginSigningSSHKeyPath, cfg.PluginSigningSSHPublicCertPath, ssh.InsecureIgnoreHostKey())
	if err != nil {
		return nil, fmt.Errorf("failed to setup client config err=%w", err)
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%v:22", cfg.PluginSigningSSHHost), &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to setup client config err=%w", err)
	}

	sftp, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to setup sftp client err=%w", err)
	}

	return sftp, nil
}

// hasAllPlatformBinaries verifies if plugin tar contains 3 platform binaries.
func hasAllPlatformBinaries(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzf)
	serverDist := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
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
		return fmt.Errorf("plugin tar contains %+v, but should contain all platform binaries", serverDist)
	}

	return nil
}

// archiveContains returns filenames that matches a given string.
func archiveContains(filePath string, contains string) ([]string, error) {
	var result []string
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive %s, err=%w", filePath, err)
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
			return nil, fmt.Errorf("failed to read next %s, err=%w", filePath, err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			if strings.Contains(header.Name, contains) {
				result = append(result, filepath.Base(header.Name))
			}
		}
	}

	return result, nil
}

// privateKey Loads a private and public key from "path" and returns a SSH ClientConfig to authenticate with the server
func privateKey(username string, path string, certPath string, keyCallBack ssh.HostKeyCallback) (ssh.ClientConfig, error) {
	privateKey, err := ioutil.ReadFile(path)

	if err != nil {
		return ssh.ClientConfig{}, err
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return ssh.ClientConfig{}, err
	}

	// Load the certificate if present
	if certPath != "" {
		cert, err := ioutil.ReadFile(certPath)
		if err != nil {
			return ssh.ClientConfig{}, err
		}

		pk, _, _, _, err := ssh.ParseAuthorizedKey(cert)
		if err != nil {
			return ssh.ClientConfig{}, err
		}

		signer, err = ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
		if err != nil {
			return ssh.ClientConfig{}, err
		}
	}

	return ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: keyCallBack,
		Timeout:         30 * time.Second,
	}, nil
}
