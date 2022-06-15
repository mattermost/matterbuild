# 1. Gitlab Pipeline Trigger

Date: 2022-06-15

## Status

Accepted

## Context

We are using Gitlab and many pipelines to automate common tasks like updating cloud servers, preparing release branches.
At the moment those need to be triggered by priviledged users by GitLab UI and/or Rest API.

Matterbuild has same support enabled for Jenkins. We want to enable that feature for Gitlab.

## Decision

We will introduce `trigger` command to trigger any pipeline in the Gitlab. That command should accept arguments to be passed 
into pipelines. Sample command:

`/mb trigger UpdateCloudServers dry_run=no`


To support this feature we decide to follow below decisions:

### Gitlab Configuration

1. Create a bot account to trigger pipelines with limited permission scheme.
2. Create a Pipeline token belongs to that bot account to trigger desired pipeline.

### Configurable Pipelines

We will have dynamic configuration and pipeline definitions. Since triggering pipelines will be done via REST api, it is possible 
to have generic code. Requirements:

1. We should not allow every pipeline to be triggered from users.
2. Every pipeline should have different token and different set of allowed users.
3. Configuration shall support hard-coded and user-defined pipeline variables.

### Matterbuild Pipeline Configuration

We will add `PipelineTriggers` object to the matterbuild configuration. Every gitlab pipeline should be defined in that pipeline as a property. Sample:

```yaml
"PipelineTriggers": {
    "UpdateCloudTestServers": { # Pipeline Name. Sample execution /mb trigger UpdateCloudServers dry_run=no
      "Description": "Generates new image from cloud branch and deploy into test servers.", # Help text of the command
      "URL": "https://git.internal.mattermost.com/api/v4/projects/163/trigger/pipeline", # Full url of the Gitlab trigger
      "Token" :"TOKEN", # Gitlab Trigger Token
      "Reference" : "cloud", # Gitlab Branch
      "Variables" : { # Hard coded or dynamic variables
        "UPDATE_CLOUD_SERVERS": "yes",  # Hard coded pipeline variable, pass it directly to the Gitlab trigger
        "DRY_RUN":"%%dry_run" # Dynamic pipeline variable, get value from user provided arguments 
      },
      "Users" : {
        "user.name.one":"user.id.one", # User name of the mattermost user, internal mattermost user id
        "user.name.two":"user.id.two",
      }
    }
  }
```

Important: To get any variable from user and slash command use `%%` as a prefix in variable definition. For instance if `DRY_RUN` variable need to be parsed from slash command execution, define that variable as below:

```
"Variables" : { 
    "UPDATE_CLOUD_SERVERS": "yes", 
    "DRY_RUN":"%%dry_run" 
}
```
Code will pass `UPDATE_CLOUD_SERVERS` to the pipeline with hard coded `yes` value, and use value from slash command arguments for `DRY_RUN` value. Using `%%` and `"%%dry_run"` as value, command will use value of `dry_run` value which is provided by user at slash command. To bind that variable, slash command need to be 

```
 /mb trigger UpdateCloudServers dry_run=no
 ```

 So code will map `no` to `DRY_RUN` pipeline variable and pass it to the pipeline.

    
## Consequences

By Using configuration file for the pipeline definition, we need to modify configuration and deploy matterbuild everytime. 

Any argument value which contains space is not supported. We assume that will not be needed and if it is we need to apply patch to the code base and redefine argument format.
