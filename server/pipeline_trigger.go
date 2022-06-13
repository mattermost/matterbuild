// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

func getPipelineFormData(pipelineTrigger *PipelineTrigger, args []string) url.Values {
	data := url.Values{}
	data.Add("token", pipelineTrigger.Token)
	data.Add("ref", pipelineTrigger.Reference)
	for key, value := range pipelineTrigger.Variables {
		var v = value
		if search := strings.TrimPrefix(value, "%%"); search != value {
			for _, r := range args {
				if replacement := strings.TrimPrefix(r, search+"="); replacement != r {
					v = replacement
					break
				}
			}
		}
		data.Add(fmt.Sprintf("variables[%s]", key), v)
	}
	return data
}

func post(url string, formData url.Values) (map[string]interface{}, error) {
	response, err := http.PostForm(url, formData)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Invalid request = %s,%s", response.Status, string(responseData))
	}

	var result map[string]interface{}
	json.Unmarshal(responseData, &result)
	return result, nil
}

func TriggerPipeline(pipelineTrigger *PipelineTrigger, args []string) (string, error) {
	formData := getPipelineFormData(pipelineTrigger, args)

	result, err := post(pipelineTrigger.URL, formData)
	if err != nil {
		return "", err
	}
	url, ok := result["web_url"]
	if !ok {
		return "", errors.New("web_url is missing at trigger pipeline response")
	}
	return url.(string), nil
}
