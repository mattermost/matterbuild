// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func getPipelineFormData(pipelineTrigger *PipelineTrigger, args []string) url.Values {
	data := url.Values{}
	data.Add("token", pipelineTrigger.Token)
	data.Add("ref", pipelineTrigger.Reference)
	for variableName, variableValue := range pipelineTrigger.Variables {
		if search := strings.TrimPrefix(variableValue, "%%"); search != variableValue {
			for _, argValue := range args {
				if replacement := strings.TrimPrefix(argValue, search+"="); replacement != argValue {
					variableValue = replacement
					break
				}
			}
		} else if search := strings.TrimPrefix(variableValue, "--"); search != variableValue {
			for _, argValue := range args {
				if variableValue == argValue {
					variableValue = "true"
					break
				}
			}
			if variableValue != "true" {
				variableValue = "false"
			}
		}
		data.Add(fmt.Sprintf("variables[%s]", variableName), variableValue)
	}
	return data
}

func post(url string, formData url.Values) (map[string]interface{}, error) {
	response, err := http.PostForm(url, formData)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("invalid request = %s,%s", response.Status, string(responseData))
	}

	var result map[string]interface{}

	if err := json.Unmarshal(responseData, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func validateArguments(args []string) error {
	if args == nil {
		return errors.New("undefined arguments")
	}
	for _, argValue := range args {
		if !strings.Contains(argValue, "=") && !strings.HasPrefix(argValue, "--") {
			return fmt.Errorf("arguments should be defined as key value pair. expected key=value, got %s", argValue)
		}
	}
	return nil
}

func TriggerPipeline(pipelineTrigger *PipelineTrigger, args []string) (string, error) {
	if err := validateArguments(args); err != nil {
		return "", err
	}

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
