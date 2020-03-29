package server

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func SetLatestURL(release string) error {

	s3BucketName := "latest-test.mattermost.com"
	DesktopVersion := "4.4.0"
	ServerVersion := release
	// if len(os.Args) > 3 {
	// 	s3BucketName = os.Args[1]
	// 	DesktopVersion = os.Args[2]
	// 	ServerVersion = os.Args[3]
	// } else {
	// 	exitErrorf("Unable to set version information: use -> latest <BucketName> <DesktopVersion> <ServerVersion>")
	// }

	// Create S3 service client
	sessionAWS, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1")},
	)
	if err != nil {
		exitErrorf("Unable to get AWS Session Info version: Please set the AWS_PROFILE environment variable.")
		return err
	}
	svc := s3.New(sessionAWS)
	bucket := s3BucketName

	// Create SetBucketWebsite parameters based on CLI input
	params := s3.PutBucketWebsiteInput{
		Bucket: aws.String(bucket),
		WebsiteConfiguration: &s3.WebsiteConfiguration{
			IndexDocument: &s3.IndexDocument{
				Suffix: aws.String("index.html"),
			},
			RoutingRules: []*s3.RoutingRule{
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("server-linux-enterprise"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String(ServerVersion + "/mattermost-enterprise-" + ServerVersion + "-linux-amd64.tar.gz"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("server-windows-enterprise"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String(ServerVersion + "/mattermost-enterprise-" + ServerVersion + "-windows-amd64.zip"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("server-linux-team"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String(ServerVersion + "/mattermost-team-" + ServerVersion + "-linux-amd64.tar.gz"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("server-windows-team"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String(ServerVersion + "/mattermost-team-" + ServerVersion + "-windows-amd64.zip"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("desktop-exe"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String("desktop/" + DesktopVersion + "/mattermost-desktop-setup-" + DesktopVersion + "-win.exe"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("desktop-dmg"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String("desktop/" + DesktopVersion + "/mattermost-desktop-" + DesktopVersion + "-mac.dmg"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("desktop-msi"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String("desktop/" + DesktopVersion + "/mattermost-desktop-" + DesktopVersion + "-x64.msi"),
					},
				},
				&s3.RoutingRule{
					Condition: &s3.Condition{
						KeyPrefixEquals: aws.String("desktop-deb"),
					},
					Redirect: &s3.Redirect{
						Protocol:       aws.String("https"),
						HostName:       aws.String("releases.mattermost.com"),
						ReplaceKeyWith: aws.String("desktop/" + DesktopVersion + "/mattermost-desktop-setup-" + DesktopVersion + "-amd64.deb"),
					},
				},
			},
		},
	}

	// Set the website configuration on the bucket. Replacing any existing
	// configuration.
	_, err = svc.PutBucketWebsite(&params)
	if err != nil {
		exitErrorf("Unable to set bucket %q website configuration, %v",
			bucket, err)
	}

	return nil

}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
