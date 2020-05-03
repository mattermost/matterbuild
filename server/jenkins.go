// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/bndr/gojenkins"
)

type JenkinsStatus struct {
	Status   string
	Duration int64
	Color    string
}

func getJenkins(jenkinsUser, jenkinsToken, jenkinsURL string) (*gojenkins.Jenkins, *AppError) {
	jenkins, err := gojenkins.CreateJenkins(jenkinsURL, jenkinsUser, jenkinsToken).Init()
	if err != nil {
		return nil, NewError("Unable to connect to jenkins!", err)
	}
	return jenkins, nil
}

// CutRelease run the Jenkins job to cut the release
func CutRelease(release string, rc string, isFirstMinorRelease bool, backportRelease bool,
	isDryRun bool, legacy bool, server string, webapp string) *AppError {
	var jobName string
	if legacy {
		jobName = Cfg.ReleaseJobLegacy
	} else {
		jobName = Cfg.ReleaseJob
	}

	isRunning, err := IsCutReleaseRunning(jobName)
	if err != nil {
		return err
	}
	if isRunning {
		return NewError("There is a release job running.", nil)
	}

	shortRelease := release[:len(release)-2]
	releaseBranch := "release-" + shortRelease
	fullRelease := release + "-" + rc
	rcpart := rc
	if rc == "" {
		rcpart = ""
		fullRelease = release
	} else {
		rcpart = "-" + rc
	}

	isFirstMinorReleaseStr := "false"
	if isFirstMinorRelease {
		isFirstMinorReleaseStr = "true"
	}

	isDryRunStr := "false"
	if isDryRun {
		isDryRunStr = "true"
	}

	isDotReleaseStr := "false"
	if backportRelease {
		isDotReleaseStr = "true"
	}

	parameters := map[string]string{
		"MM_VERSION":             release,
		"MM_RC":                  rcpart,
		"IS_FIRST_MINOR_RELEASE": isFirstMinorReleaseStr,
		"IS_DRY_RUN":             isDryRunStr,
		"IS_DOT_RELEASE":         isDotReleaseStr,
		"IS_BACKPORT":            isDotReleaseStr,
		"PIP_BRANCH":             releaseBranch,
	}

	if server != "" {
		parameters["MM_BUILDER_SERVER_DOCKER"] = server
	}

	if webapp != "" {
		parameters["MM_BUILDER_WEBAPP_DOCKER"] = webapp
	}

	// We want to return so the user knows the build has started.
	// Build jobs should report their own failure.
	go func() {
		result, err := RunJobWaitForResult(
			jobName,
			parameters)
		if err != nil || result != gojenkins.STATUS_SUCCESS {
			LogError("Release Job failed. Version=" + fullRelease + " err= " + err.Error() + " Jenkins result= " + result)
			return
		} else {
			// If Release was success trigger the Rctesting job to update
			LogInfo("Release Job Status: " + result)
			if !backportRelease {
				LogInfo("Will trigger Job: " + Cfg.RCTestingJob)
				RunJobParameters(Cfg.RCTestingJob, map[string]string{"LONG_RELEASE": fullRelease}, Cfg.CIServerJenkinsUserName, Cfg.CIServerJenkinsToken, Cfg.CIServerJenkinsURL)

				// Only update the CI servers and community if this is the latest release
				LogInfo("Setting CI Servers")
				SetCIServerBranch(releaseBranch)

			}
		}
	}()

	return nil
}

func getJob(name, jenkinsUser, jenkinsToken, jenkinsURL string) (*gojenkins.Job, *AppError) {
	jenkins, err := getJenkins(jenkinsUser, jenkinsToken, jenkinsURL)

	if err != nil {
		LogError("[getJob] Unable to get Jenkins. err=" + err.Error())
		return nil, err
	}

	LogInfo("[getJob] Job Name: " + name)
	if job, err := jenkins.GetJob(name); err != nil {
		LogError("[getJob] Unable to get job: " + name + " err=" + err.Error())
		return nil, NewError("Unable to get job", err)
	} else {
		return job, nil
	}

}

func GetJobConfig(name, jenkinsUser, jenkinsToken, jenkinsURL string) (string, *AppError) {
	if job, err := getJob(name, jenkinsUser, jenkinsToken, jenkinsURL); err != nil {
		LogError("[GetJobConfig] Unable to get the Job: " + name + " err=" + err.Error())
		return "", err
	} else {
		if config, err := job.GetConfig(); err != nil {
			LogError("[GetJobConfig] Unable to get job config for job: " + name + " err=" + err.Error())
			return "", NewError("Unable to get job config", err)
		} else {
			return config, nil
		}
	}
}

func SaveJobConfig(name string, config string) *AppError {
	job, err := getJob(name, Cfg.CIServerJenkinsUserName, Cfg.CIServerJenkinsToken, Cfg.CIServerJenkinsURL)
	if err != nil {
		LogError("[SaveJobConfig] Unable to save job config for job: " + name + " err=" + err.Error())
		return err
	}

	errUpdate := job.UpdateConfig(config)
	if errUpdate != nil {
		LogError("[SaveJobConfig] Unable to update job config for job: " + name + " err=" + errUpdate.Error())
		return NewError("Unable to update job config", errUpdate)
	}

	return nil
}

func SetCIServerBranch(branch string) *AppError {
	for _, serverjob := range Cfg.CIServerJobs {
		LogInfo("[SetCIServerBranch] Setting branch " + branch + " to " + serverjob)
		config, err := GetJobConfig(serverjob, Cfg.CIServerJenkinsUserName, Cfg.CIServerJenkinsToken, Cfg.CIServerJenkinsURL)
		if err != nil {
			LogError("[SetCIServerBranch] Error getting the job config for" + serverjob + " err=" + err.Error())
			return err
		}

		LogInfo("Config before changes" + config)
		config = strings.Replace(config, "version='1.1'", "version='1.0'", 1)
		config = strings.Replace(config, "version=\"1.1\"", "version=\"1.0\"", 1)
		jConfig := etree.NewDocument()
		if err := jConfig.ReadFromString(config); err != nil {
			LogError("[SetCIServerBranch] Unable to read job configuration for " + serverjob + " err=" + err.Error())
			return NewError("Unable to read job configuration for "+serverjob, err)
		}

		// Change branch to build from
		element := jConfig.Root().FindElement("./properties/hudson.model.ParametersDefinitionProperty/parameterDefinitions/hudson.model.StringParameterDefinition/defaultValue")
		if element == nil {
			LogError("[SetCIServerBranch] Unable to correct default branch element for " + serverjob)
			return NewError("Unable to correct default branch element for "+serverjob, nil)
		}
		element.SetText(branch)

		jConfigStringOut, errConfig := jConfig.WriteToString()
		if errConfig != nil {
			LogError("[SetCIServerBranch] Unable to write out final job config for " + serverjob + " err=" + errConfig.Error())
			return NewError("Unable to write out final job config for "+serverjob, errConfig)
		}

		jConfigStringOut = strings.Replace(jConfigStringOut, "version=\"1.0\"", "version=\"1.1\"", 1)
		jConfigStringOut = strings.Replace(jConfigStringOut, "version='1.0'", "version='1.1'", 1)
		LogInfo("Config after changes" + jConfigStringOut)
		if err := SaveJobConfig(serverjob, jConfigStringOut); err != nil {
			LogError("[SetCIServerBranch] Unable to save job for " + serverjob + " err=" + err.Error())
			return NewError("Unable to save job for "+serverjob, err)
		}

	}

	return nil
}

func RunJob(name string) *AppError {
	LogInfo("Running Job: " + name)
	return RunJobParameters(name, nil, Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
}

func RunJobWaitForResult(name string, parameters map[string]string) (string, *AppError) {
	job, err := getJob(name, Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
	if err != nil {
		LogError("[RunJobWaitForResult] Did not find Job: " + name + " err=" + err.Error())
		return "", err
	}

	newBuildNumber := job.Raw.NextBuildNumber

	_, err2 := job.InvokeSimple(parameters)
	if err2 != nil {
		LogError("[RunJobWaitForResult] Unable to envoke job " + " err=" + err.Error())
		return "", NewError("Unable to envoke job.", err)
	}

	var err3 error
	var status int
	tries := 1
	build := gojenkins.Build{
		Jenkins: job.Jenkins,
		Job:     job,
		Raw:     new(gojenkins.BuildResponse),
		Depth:   1,
		Base:    "/job/" + name + "/" + strconv.FormatInt(newBuildNumber, 10),
	}
	status, err3 = build.Poll()

	for ; err3 != nil || status != 200; tries += 1 {
		status, err3 = build.Poll()
		if tries >= 5 {
			LogError("[RunJobWaitForResult] Unable to get build for pre-checks job: " + strconv.Itoa(int(newBuildNumber)) + " err=" + err3.Error())
			return "", NewError("Unable to get build for pre-checks job: "+strconv.Itoa(int(newBuildNumber)), err3)
		}
		time.Sleep(time.Second * time.Duration(tries))
	}

	// Wait for the build to finish
	time.Sleep(time.Second * 5)
	build.Poll()
	for build.IsRunning() {
		LogInfo("[RunJobWaitForResult] Waiting for job: " + name + " to complete")
		time.Sleep(time.Second * 30)
		build.Poll()
	}

	return build.GetResult(), nil
}

func RunJobParameters(name string, parameters map[string]string, jenkinsUser, jenkinsPassword, jenkinsURL string) *AppError {
	if job, err := getJob(name, jenkinsUser, jenkinsPassword, jenkinsURL); err != nil {
		return err
	} else {
		_, err2 := job.InvokeSimple(parameters)
		if err2 != nil {
			LogError("[RunJobParameters] Unable to envoke job. err=" + err.Error())
			return NewError("Unable to envoke job.", err)
		}
	}

	return nil
}

func IsCutReleaseRunning(name string) (bool, *AppError) {
	job, err := getJob(name, Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
	if err != nil {
		LogError("[IsCutReleaseRunning] Did not find Job: " + name + " err=" + err.Error())
		return false, err
	}

	build, err1 := job.GetLastBuild()
	if err1 != nil {
		LogError("[IsCutReleaseRunning] Error getting the last build for: " + name + " err=" + err1.Error())
		return false, NewError("Unable to get last build", err1)
	}

	if build.IsRunning() {
		return true, nil
	}

	return false, nil
}

func GetLatestResult(name string) (*JenkinsStatus, *AppError) {
	buildStatus := &JenkinsStatus{}
	job, err := getJob(name, Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
	if err != nil {
		LogError("[GetLatestResult] Did not find Job: " + name + " err=" + err.Error())
		return nil, err
	}

	build, err1 := job.GetLastBuild()
	if err1 != nil {
		LogError("[GetLatestResult] Error getting the last build for: " + name + " err=" + err1.Error())
		return nil, NewError("Unable to get last build", err1)
	}

	if build.IsRunning() {
		buildStatus.Status = "Running"
		buildStatus.Duration = 0
		buildStatus.Color = "#0060aa"
	} else {
		buildStatus.Duration = build.GetDuration()
		buildStatus.Status = build.GetResult()
		if buildStatus.Status == gojenkins.STATUS_SUCCESS {
			buildStatus.Color = "#86c323"
		} else {
			buildStatus.Color = "#e20025"
		}
	}

	return buildStatus, nil
}

func GetJenkinsArtifacts(jobname string) ([]gojenkins.Artifact, *AppError) {
	job, err := getJob(jobname, Cfg.JenkinsUsername, Cfg.JenkinsPassword, Cfg.JenkinsURL)
	if err != nil {
		LogError("[GetJenkinsArtifact] Did not find Job: " + jobname + " err=" + err.Error())
		return nil, err
	}

	build, err1 := job.GetLastBuild()
	if err1 != nil {
		LogError("[GetJenkinsArtifact] Error getting the last build for: " + jobname + " err=" + err1.Error())
		return nil, NewError("Unable to get last build", err1)
	}

	artifacts := build.GetArtifacts()
	if len(artifacts) == 0 {
		LogError("[GetJenkinsArtifact] No artifacts returned: " + jobname)
		return nil, NewError("No artifacts returned", nil)
	}

	for _, a := range artifacts {
		a.SaveToDir("/tmp")
	}

	return artifacts, nil
}
