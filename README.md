# Matterbuild

Matterbuild is an internal Mattermost chatops tool for generating Mattermost releases.

## Get Involved

- [Join the discussion on ~Developers: DevOps](https://community.mattermost.com/core/channels/build)

## Developing

### Environment Setup

Essentials:

1. Install [Go](https://golang.org/doc/install)
2. `brew install gnu-tar` for macOS

Optionals:

1. [Tilt](https://tilt.dev/) (to deploy on a local dev K8s cluster)
2. [kind](https://kind.sigs.k8s.io/) (to spin up a local dev K8s cluster)
3. [kustomize](https://github.com/kubernetes-sigs/kustomize)

### Running

This project uses `tilt` to deploy to local Kubernetes cluster. In order to do this you need a local Kuberetes cluster (`kind` is recommended).

```bash
kind create cluster --name matterbuild
```

Matterbuild deployment to any cluster and any environment (dev, prod, etc) depends on existense of `deploy/config/confog.json` file, this file is `.gitignore`d and you cn safeley choose to copy sample config there for local development and testing:

```shell
cp config.json deploy/config/
```

Point `KUBECONFIG` to the newly created cluster, and start `tilt` and open [http://localhost:8080/](http://localhost:8080/):

```shell
make run
```

**Note:** If you don't want to use Tilt or deploy to local cluster you can ignore it and simply start the binary server:

```bash
NOTILT=1 make run
```

### Testing

Running all tests:

```shell
make test
```

Generate github mocks:

```shell
make mocks
```

### Setting up slash command in Mattermost

1. Navigate to http://localhost:8065/_redirect/integrations/commands/add
2. Set Command Trigger Word to `matterbuild`
3. Set Request URL to `http://localhost:5001/slash_command`
4. Set Request Method to `POST`
5. Click `Save`
6. Navigate to any channel and type `/matterbuild cutplugin --tag v0.6.3 --repo mattermost-plugin-demo --commitSHA 24dbd65762612fb72af6e7c30b40e9e8d0a90968`

### Test via curl

Invoke matterbuild commands using curl:

```shell
curl -X POST http://localhost:5001/slash_command -d "command=/matterbuild&token=&user_id=" -d "text=cutplugin+--tag+v0.4.1+--repo+mattermost-plugin-demo" 
```

### Testing cutplugin

To test the cutplugin you have to:

1. Connect to [Mattermost VPN](https://developers.mattermost.com/internal/infrastructure/vpn/)
2. Get AWS [Vault](https://developers.mattermost.com/internal/infrastructure/vault/) credentials
3. Signed public certificate by Vault
4. Generate Github Token
5. Set following fields in `config.json` before running matterbuild

```json
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
