// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
)

// Get the existing wensite config if any exists
func getBucketConfig(svc *s3.S3, bucket string) (*s3.GetBucketWebsiteOutput, error) {

	input := &s3.GetBucketWebsiteInput{
		Bucket: aws.String(bucket),
	}

	result, err := svc.GetBucketWebsite(input)
	if err != nil {
		if err, ok := err.(awserr.Error); ok {
			switch err.Code() {
			default:
				LogError(err.Error())
			}
		} else {
			LogError(err.Error())
		}
		return nil, err
	}
	return result, nil
}

func checkBucket(svc *s3.S3, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	result, err := svc.ListObjectsV2(input)
	return result, err
}

func checkIfBucketExistsWithPrefixAndWait(ctx context.Context, svc *s3.S3, cfg *MatterbuildConfig, ver string, typeToRelease string) (*s3.ListObjectsV2Output, error) {

	releaseBucket := cfg.S3ReleaseBucket
	s3Prefix := ver + "/"
	if typeToRelease == "desktop" {
		s3Prefix = "desktop/" + ver + "/"
	}
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(releaseBucket),
		Prefix: aws.String(s3Prefix),
	}

	LogInfo("Checking for s3 bucket: %s", releaseBucket+"/"+s3Prefix)
	result, _ := checkBucket(svc, input)
	if *result.KeyCount != int64(0) {
		return result, nil
	}

	for {
		select {
		case <-ctx.Done():
			LogError("Timed out waiting for %s to be created", releaseBucket+"/"+s3Prefix)
			return nil, ctx.Err()
		case <-time.After(5 * time.Minute):
			result, _ = checkBucket(svc, input)
			if *result.KeyCount != int64(0) {
				return result, nil
			}
			timeLeft, _ := ctx.Deadline()
			LogInfo("Release %s is not available; waiting until: %s", releaseBucket+"/"+s3Prefix, timeLeft.Format("15:04:05"))
		}
	}
}

func preserverExistingRoutingRules(svc *s3.S3, cfg *MatterbuildConfig, typeToRelease string, params s3.PutBucketWebsiteInput) error {

	bucketConfig, err := getBucketConfig(svc, cfg.S3BucketNameForLatestURLs)
	if err != nil {
		LogError("Unable to get the %s AWS Bucket Website Config.", cfg.S3BucketNameForLatestURLs)
		return err
	}

	// Check for Routing Rules that are not related to the typeToRelease value and carry them forward
	for _, value := range bucketConfig.RoutingRules {
		checked := false

		if typeToRelease == "desktop" {
			checked = strings.Contains(*value.Condition.KeyPrefixEquals, "enterprise") || strings.Contains(*value.Condition.KeyPrefixEquals, "team")
		}

		if typeToRelease == "server" {
			checked = strings.Contains(*value.Condition.KeyPrefixEquals, "desktop")
		}

		if checked {
			params.WebsiteConfiguration.RoutingRules = append(params.WebsiteConfiguration.RoutingRules, value)
			LogInfo("Copying rule %s forward as it is not being updated", *value.Condition.KeyPrefixEquals)
		}
	}

	return nil
}

func addRoutingRule(file string, keyToUse string, params s3.PutBucketWebsiteInput, suffix string) error {
	valueToAdd := &s3.RoutingRule{
		Condition: &s3.Condition{
			KeyPrefixEquals: aws.String(keyToUse + suffix),
		},
		Redirect: &s3.Redirect{
			Protocol:       aws.String("https"),
			HostName:       aws.String("releases.mattermost.com"),
			ReplaceKeyWith: aws.String(file),
		},
	}
	LogInfo("Adding %s RoutingRule for: %s", keyToUse+suffix, file)
	params.WebsiteConfiguration.RoutingRules = append(params.WebsiteConfiguration.RoutingRules, valueToAdd)

	return nil
}

func generateNewRoutesForRelease(result *s3.ListObjectsV2Output, fileSearchValue string, ver string, params s3.PutBucketWebsiteInput) error {

	for _, value := range result.Contents {
		if strings.Contains(*value.Key, fileSearchValue) && !strings.Contains(*value.Key, ".sig") {
			switchValue := *value.Key
			switch {
			case strings.HasSuffix(switchValue, ".dmg"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-dmg")
			case strings.HasSuffix(switchValue, ".exe"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-exe")
			case strings.HasSuffix(switchValue, "amd64.deb"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-amd64-deb")
			case strings.HasSuffix(switchValue, "i386.deb"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-i386-deb")
			case strings.HasSuffix(switchValue, "x86_64.AppImage"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-x86_64-appimage")
			case strings.HasSuffix(switchValue, "i386.AppImage"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-i386-appimage")
			case strings.HasSuffix(switchValue, "x64.msi"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-x64-msi")
			case strings.HasSuffix(switchValue, "x86.msi"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-x86-msi")
			case strings.HasSuffix(switchValue, ver+"-linux-ia32.tar.gz"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-ia32-linux-tar")
			case strings.HasSuffix(switchValue, ver+"-linux-x64.tar.gz"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-x64-linux-tar")
			case strings.HasSuffix(switchValue, ver+"-linux-amd64.tar.gz"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-linux")
			case strings.HasSuffix(switchValue, ver+"-windows-amd64.zip"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-windows")
			case strings.HasSuffix(switchValue, ver+"-osx-amd64.tar.gz"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-osx")
			}

		}
	}

	return nil
}

func generateURLTextFile(cfg *MatterbuildConfig, params *s3.PutBucketWebsiteInput) (string, error) {
	txtToReturn := ""
	maxLen := len(params.WebsiteConfiguration.RoutingRules)
	for len, value := range params.WebsiteConfiguration.RoutingRules {
		txtToReturn += "https://" + cfg.S3BucketNameForLatestURLs + "/" + *value.Condition.KeyPrefixEquals
		if len != maxLen-1 {
			txtToReturn += "\n"
		}
	}

	return txtToReturn, nil
}

func uploadIndexFile(awsSession client.ConfigProvider, cfg *MatterbuildConfig, txtFile string) error {
	uploader := s3manager.NewUploader(awsSession)
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(cfg.S3BucketNameForLatestURLs),
		Key:         aws.String("index.html"),
		Body:        strings.NewReader(txtFile),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to upload file, index.html")
	}
	LogInfo("File uploaded to, %s\n", result.Location)

	return nil
}

// SetLatestURL updates the S3 website routing configuration
func SetLatestURL(typeToRelease string, ver string, cfg *MatterbuildConfig) error {

	creds := credentials.NewStaticCredentials(cfg.S3LatestAWSAccessKey, cfg.S3LatestAWSSecretKey, "")
	awsCfg := aws.NewConfig().WithRegion(cfg.S3LatestAWSRegion).WithCredentials(creds)
	awsSession := session.Must(session.NewSession(awsCfg))
	svc := s3.New(awsSession)

	params := s3.PutBucketWebsiteInput{
		Bucket: aws.String(cfg.S3BucketNameForLatestURLs),
		WebsiteConfiguration: &s3.WebsiteConfiguration{
			IndexDocument: &s3.IndexDocument{
				Suffix: aws.String("index.html"),
			},
			RoutingRules: []*s3.RoutingRule{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	result, err := checkIfBucketExistsWithPrefixAndWait(ctx, svc, cfg, ver, typeToRelease)
	if err != nil {
		return err
	}

	err = preserverExistingRoutingRules(svc, cfg, typeToRelease, params)
	if err != nil {
		return err
	}

	generateNewRoutesForRelease(cfg, result, "mattermost-enterprise", ver, params)
	if err != nil {
		return err
	}

	generateNewRoutesForRelease(cfg, result, "mattermost-desktop", ver, params)
	if err != nil {
		return err
	}

	generateNewRoutesForRelease(cfg, result, "mattermost-team", ver, params)
	if err != nil {
		return err
	}

	txtFile, err := generateURLTextFile(cfg, &params)
	if err != nil {
		return err
	}
	fmt.Println(txtFile)
	uploadIndexFile(awsSession, cfg, txtFile)

	_, err = svc.PutBucketWebsite(&params)
	if err != nil {
		LogError("Unable to set bucket %q website configuration, %v", cfg.S3BucketNameForLatestURLs, err)
	}

	return nil

}
