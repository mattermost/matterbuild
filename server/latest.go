// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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

func checkIfBucketExistsWithPrefixAndWait(svc *s3.S3, cfg *MatterbuildConfig, ver string, typeToRelease string) (*s3.ListObjectsV2Output, error) {

	releaseBucket := "releases.mattermost.com"
	s3Prefix := ver + "/"
	if typeToRelease == "desktop" {
		s3Prefix = "desktop/" + ver + "/"
	}
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(releaseBucket),
		Prefix: aws.String(s3Prefix),
	}
	result, err := svc.ListObjectsV2(input)
	if err != nil {
		if err, ok := err.(awserr.Error); ok {
			switch err.Code() {
			default:
				fmt.Println(err.Error())
				return nil, err
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
			return nil, err
		}
	} else {
		if *result.KeyCount == int64(0) {
			LogInfo("Key count is 0")
		}
	}

	return result, err
}

func preserverExistingRoutingRules(svc *s3.S3, cfg *MatterbuildConfig, typeToRelease string, params s3.PutBucketWebsiteInput) error {

	bucketConfig, err := getBucketConfig(svc, cfg.S3BucketNameForLatestURLs)
	if err != nil {
		LogError("Unable to get the %s AWS Bucket Website Config.", cfg.S3BucketNameForLatestURLs)
		return err
	}

	valueToCheck := typeToRelease
	if typeToRelease == "server" {
		valueToCheck = "enterprise"
	}

	// Check for Routing Rules that are not related to the typeToRelease value and carry them forward
	for _, value := range bucketConfig.RoutingRules {
		if !strings.Contains(*value.Condition.KeyPrefixEquals, valueToCheck) {
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

func generateNewRoutesForRelease(svc *s3.S3, cfg *MatterbuildConfig, result *s3.ListObjectsV2Output, fileSearchValue string, ver string, params s3.PutBucketWebsiteInput) error {

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
			case strings.Contains(switchValue, ver+"-linux-amd64.tar.gz"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-linux")
			case strings.Contains(switchValue, ver+"-windows-amd64.zip"):
				addRoutingRule(*value.Key, fileSearchValue, params, "-windows")
			case strings.Contains(switchValue, ver+"-osx-amd64.tar.gz"):
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
		txtToReturn += "http://" + cfg.S3BucketNameForLatestURLs + "/" + *value.Condition.KeyPrefixEquals
		if len != maxLen-1 {
			txtToReturn += "\n"
		}
	}

	return txtToReturn, nil
}

// SetLatestURL updates the S3 website routing configuration
func SetLatestURL(typeToRelease string, ver string, cfg *MatterbuildConfig) error {

	// User AWS Creds from the config.json
	creds := credentials.NewStaticCredentials(cfg.S3LatestAWSAccessKey, cfg.S3LatestAWSSecretKey, "")
	awsCfg := aws.NewConfig().WithRegion(cfg.S3LatestAWSRegion).WithCredentials(creds)
	awsSession := session.Must(session.NewSession(awsCfg))
	svc := s3.New(awsSession)

	// Create SetBucketWebsite parameter structure
	params := s3.PutBucketWebsiteInput{
		Bucket: aws.String(cfg.S3BucketNameForLatestURLs),
		WebsiteConfiguration: &s3.WebsiteConfiguration{
			IndexDocument: &s3.IndexDocument{
				Suffix: aws.String("index.html"),
			},
			RoutingRules: []*s3.RoutingRule{},
		},
	}

	result, err := checkIfBucketExistsWithPrefixAndWait(svc, cfg, ver, typeToRelease)
	if err != nil {
		return err
	}

	err = preserverExistingRoutingRules(svc, cfg, typeToRelease, params)
	if err != nil {
		return err
	}

	generateNewRoutesForRelease(svc, cfg, result, "mattermost-enterprise", ver, params)
	if err != nil {
		return err
	}

	generateNewRoutesForRelease(svc, cfg, result, "mattermost-desktop", ver, params)
	if err != nil {
		return err
	}

	txtFile, err := generateURLTextFile(cfg, &params)
	if err != nil {
		return err
	}
	fmt.Println(txtFile)

	// Set the website configuration on the bucket. Replacing any existing
	// configuration.
	_, err = svc.PutBucketWebsite(&params)
	if err != nil {
		LogError("Unable to set bucket %q website configuration, %v", cfg.S3BucketNameForLatestURLs, err)
	}

	return nil

}