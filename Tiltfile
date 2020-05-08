docker_build('mattermost/matterbuild', '.', dockerfile='Dockerfile')

k8s_yaml([
    'deploy/rbac.yaml',
    'deploy/config.yaml',
    'deploy/secret.yaml',
    'deploy/deployment.yaml',
    'deploy/service.yaml',
])

k8s_resource('matterbuild', port_forwards=8080)
