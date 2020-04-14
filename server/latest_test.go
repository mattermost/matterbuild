// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"testing"
)

func TestSetLatestURL(t *testing.T) {
	type args struct {
		typeToRelease string
		ver           string
		cfg           *MatterbuildConfig
	}
	LoadConfig("../config.json")
	
	Cfg.S3BucketNameForLatestURLs = "latest-test.mattermost.com"
	Cfg.S3ReleaseBucket = "latest-test.mattermost.com"
	LogInfo("Starting Test for Matterbuild SetLatestURL")
	
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		//{name: "Server",args: args{"server","4.212.0",Cfg},wantErr: true},
		{name: "Server",args: args{"server","5.21.0",Cfg},wantErr: false},
		{name: "Desktop",args: args{"desktop","4.4.0",Cfg},wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetLatestURL(tt.args.typeToRelease, tt.args.ver, tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("SetLatestURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
