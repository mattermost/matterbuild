docker_build('mattermost/matterbuild', '.', dockerfile='Dockerfile')

k8s_yaml(kustomize('./deploy/overlays/dev'))

k8s_resource('matterbuild', port_forwards=8080)
