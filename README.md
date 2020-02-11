# Matterbuild

Matterbuild is an internal Mattermost chatops tool for generating Mattermost releases.

## Get Involved

- [Join the discussion on ~Developers: DevOps](https://community.mattermost.com/core/channels/build)

## Developing

### Environment Setup

1. Install [Go](https://golang.org/doc/install)
2. `brew install gnu-tar` for macOS

### Running

Simply run the following:

```
$ make run
```

### Testing

Running all tests:

```
$ make test
```

### Setting up slash command in Mattermost

1. Navigate to http://localhost:8065/_redirect/integrations/commands/add
2. Set Command Trigger Word to `matterbuild`
3. Set Request URL to `http://localhost:5001/slash_command`
4. Set Request Method to `POST`
5. Click `Save`
6. Navigate to any channel and type `/matterbuild cutPlugin --tag v0.4.1 --repo mattermost-plugin-demo`

### Test via curl

Invoke matterbuild commands using curl:

```
curl -X POST http://localhost:5001/slash_command -d "command=/matterbuild&token=&user_id=" -d "text=cutPlugin+--tag+v0.4.1+--repo+mattermost-plugin-demo" 
```

### Testing cutPlugin

To test the cutPlugin you have to:
1. Connect to [Mattermost VPN](https://developers.mattermost.com/internal/infrastructure/vpn/)
2. Get AWS [Vault](https://developers.mattermost.com/internal/infrastructure/vault/) credentials
3. Signed public certificate by Vault
4. Generate Github Token
5. Set following fields in `config.json` before running matterbuild
```
// Used to authenticate invoking slash command
"AllowedTokens": ["irkngs1z4jrcz8t9aiyzu8zx3r", ""],
"AllowedUsers": ["gcye3z5pnpgibkcfhpemsp78ey", ""],

"GithubAccessToken": "---",
"GithubOrg": "mattermost",

"PluginSigningSSHKeyPath": "/Users/<user>/.ssh/id_rsa",
"PluginSigningSSHPublicCertPath": "/Users/<user>/.ssh/signed-cert.pub",
"PluginSigningSSHUser": "---",
"PluginSigningSSHHost": "---",
"PluginSigningSSHHostPublicKey": "199.199.222.222 ecdsa-sha2-nistp256 AyNTYAAABBBDZEF6pmnR=",
"PluginSigningAWSAccessKey": "---",
"PluginSigningAWSSecretKey": "---",
"PluginSigningAWSRegion": "us-east-1",
"PluginSigningAWSS3PluginBucket": "mattermost-toolkit-dev"
```

