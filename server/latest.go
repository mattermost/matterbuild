// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
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

// SetLatestURL updates the S3 website routing configuration
func SetLatestURL(typeToRelease string, ver string, cfg *MatterbuildConfig) error {

	// User AWS Creds from the config.json
	creds := credentials.NewStaticCredentials(cfg.S3LatestAWSAccessKey, cfg.S3LatestAWSSecretKey, "")
	awsCfg := aws.NewConfig().WithRegion(cfg.S3LatestAWSRegion).WithCredentials(creds)
	awsSession := session.Must(session.NewSession(awsCfg))
	svc := s3.New(awsSession)

	bucketConfig, err := getBucketConfig(svc, cfg.S3BucketNameForLatestURLs)
	if err != nil {
		LogError("Unable to get the %s AWS Bucket Website Config.", cfg.S3BucketNameForLatestURLs)
		return err
	}

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

	// Check for Routing Rules that are not related to the typeToRelease value and carry them forward
	for _, value := range bucketConfig.RoutingRules {
		if !strings.Contains(*value.Condition.KeyPrefixEquals, typeToRelease) {
			params.WebsiteConfiguration.RoutingRules = append(params.WebsiteConfiguration.RoutingRules, value)
			LogInfo("Copying rule %s forward as it is not being updated", *value.Condition.KeyPrefixEquals)
		}
	}

	// Add updated rules based on the typeToRelease
	if typeToRelease == "server" {
		params.WebsiteConfiguration.RoutingRules = append(params.WebsiteConfiguration.RoutingRules,
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("server-linux-enterprise"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String(ver + "/mattermost-enterprise-" + ver + "-linux-amd64.tar.gz"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("server-windows-enterprise"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String(ver + "/mattermost-enterprise-" + ver + "-windows-amd64.zip"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("server-linux-team"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String(ver + "/mattermost-team-" + ver + "-linux-amd64.tar.gz"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("server-windows-team"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String(ver + "/mattermost-team-" + ver + "-windows-amd64.zip"),
				},
			},
		)
	}

	if typeToRelease == "desktop" {
		params.WebsiteConfiguration.RoutingRules = append(params.WebsiteConfiguration.RoutingRules,
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("desktop-exe"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String("desktop/" + ver + "/mattermost-desktop-setup-" + ver + "-win.exe"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("desktop-dmg"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String("desktop/" + ver + "/mattermost-desktop-" + ver + "-mac.dmg"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("desktop-msi"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String("desktop/" + ver + "/mattermost-desktop-" + ver + "-x64.msi"),
				},
			},
			&s3.RoutingRule{
				Condition: &s3.Condition{
					KeyPrefixEquals: aws.String("desktop-deb"),
				},
				Redirect: &s3.Redirect{
					Protocol:       aws.String("https"),
					HostName:       aws.String("releases.mattermost.com"),
					ReplaceKeyWith: aws.String("desktop/" + ver + "/mattermost-desktop-setup-" + ver + "-amd64.deb"),
				},
			},
		)
	}

	// Set the website configuration on the bucket. Replacing any existing
	// configuration.
	_, err = svc.PutBucketWebsite(&params)
	if err != nil {
		LogError("Unable to set bucket %q website configuration, %v", cfg.S3BucketNameForLatestURLs, err)
	}

	return nil

}
