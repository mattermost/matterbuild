// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
)

var s3ObjectOutput *s3.ListObjectsV2Output = &s3.ListObjectsV2Output{
	Contents: []*s3.Object{
		&s3.Object{
			ETag:         aws.String("\"4453bec2407cc30ab7968a1b49d37c2a-32\""),
			Key:          aws.String("5.21.0/mattermost-5.21.0-linux-amd64.tar.gz"),
			LastModified: &time.Time{},
			Size:         aws.Int64(165194772),
			StorageClass: aws.String("STANDARD"),
		}, &s3.Object{
			ETag:         aws.String("\"dc98f3008a7772c48a9dfa3eaa551d04\""),
			Key:          aws.String("5.21.0/mattermost-5.21.0-linux-amd64.tar.gz.sig"),
			LastModified: &time.Time{},
			Size:         aws.Int64(310),
			StorageClass: aws.String("STANDARD"),
		},
		&s3.Object{
			ETag:         aws.String("\"4453bec2407cc30ab7968a1b49d37c2a-32\""),
			Key:          aws.String("5.21.0/mattermost-enterprise-5.21.0-linux-amd64.tar.gz"),
			LastModified: &time.Time{},
			Size:         aws.Int64(165194772),
			StorageClass: aws.String("STANDARD"),
		},
		&s3.Object{
			ETag:         aws.String("\"4453bec2407cc30ab7968a1b49d37c2a-32\""),
			Key:          aws.String("5.21.0/mattermost-enterprise-5.21.0-linux-amd64.tar.gz.sig"),
			LastModified: &time.Time{},
			Size:         aws.Int64(310),
			StorageClass: aws.String("STANDARD"),
		},
		&s3.Object{
			ETag:         aws.String("\"4453bec2407cc30ab7968a1b49d37c2a-32\""),
			Key:          aws.String("desktop/4.4.0/mattermost-desktop-4.4.0-linux-amd64.deb"),
			LastModified: &time.Time{},
			Size:         aws.Int64(310),
			StorageClass: aws.String("STANDARD"),
		},
		&s3.Object{
			ETag:         aws.String("\"4453bec2407cc30ab7968a1b49d37c2a-31\""),
			Key:          aws.String("5.21.0/mattermost-team-5.21.0-osx-amd64.tar.gz"),
			LastModified: &time.Time{},
			Size:         aws.Int64(160596349),
			StorageClass: aws.String("STANDARD"),
		},
	},
	IsTruncated: aws.Bool(false),
	KeyCount:    aws.Int64(18),
	MaxKeys:     aws.Int64(1000),
	Name:        aws.String("releases.mattermost.com"),
	Prefix:      aws.String("5.21.0/"),
}

func Test_generateNewRoutesForRelease(t *testing.T) {
	type args struct {
		result          *s3.ListObjectsV2Output
		fileSearchValue string
		ver             string
		params          s3.PutBucketWebsiteInput
	}

	LoadConfig("../config.json")

	Cfg.S3BucketNameForLatestURLs = "latest-test.mattermost.com"
	Cfg.S3ReleaseBucket = "latest-test.mattermost.com"
	LogInfo("Starting Test for Matterbuild generateNewRoutesForRelease")

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Server",
			args: args{
				s3ObjectOutput,
				"mattermost-enterprise",
				"5.21.0",
				s3.PutBucketWebsiteInput{
					Bucket: aws.String(Cfg.S3BucketNameForLatestURLs),
					WebsiteConfiguration: &s3.WebsiteConfiguration{
						IndexDocument: &s3.IndexDocument{
							Suffix: aws.String("index.html"),
						},
						RoutingRules: []*s3.RoutingRule{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Server",
			args: args{
				s3ObjectOutput,
				"mattermost-team",
				"5.21.0",
				s3.PutBucketWebsiteInput{
					Bucket: aws.String(Cfg.S3BucketNameForLatestURLs),
					WebsiteConfiguration: &s3.WebsiteConfiguration{
						IndexDocument: &s3.IndexDocument{
							Suffix: aws.String("index.html"),
						},
						RoutingRules: []*s3.RoutingRule{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Desktop",
			args: args{
				s3ObjectOutput,
				"mattermost-desktop",
				"4.4.0",
				s3.PutBucketWebsiteInput{
					Bucket: aws.String(Cfg.S3BucketNameForLatestURLs),
					WebsiteConfiguration: &s3.WebsiteConfiguration{
						IndexDocument: &s3.IndexDocument{
							Suffix: aws.String("index.html"),
						},
						RoutingRules: []*s3.RoutingRule{},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := generateNewRoutesForRelease(tt.args.result, tt.args.fileSearchValue, tt.args.ver, tt.args.params); (err != nil) != tt.wantErr {
				t.Errorf("generateNewRoutesForRelease() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.NotEmpty(t, tt.args.params.WebsiteConfiguration.RoutingRules, "The Routing Rules should not be empty")
			for _, rr := range tt.args.params.WebsiteConfiguration.RoutingRules {
				if !checkS3Key(t, s3ObjectOutput, rr.Redirect.ReplaceKeyWith) {
					t.Errorf("generateNewRoutesForRelease() error = %s for %s", "S3 Routing Rule Key Not Found", *rr.Redirect.ReplaceKeyWith)
				}
			}
		})
	}
}

func checkS3Key(t *testing.T, s3ObjectOutput *s3.ListObjectsV2Output, valueToCheck *string) bool {
	for _, keys := range s3ObjectOutput.Contents {
		if strings.Contains(*keys.Key, *valueToCheck) {
			assert.Equal(t, keys.Key, valueToCheck)
			return true
		}
	}
	return false
}
