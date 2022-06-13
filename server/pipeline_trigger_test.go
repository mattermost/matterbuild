// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

const triggerURL = "http://localhost:8080/trigger"
const pipelineURL = "https://localhost:8080/pipeline/189"

func TestGetPipelineFormData(t *testing.T) {
	pipelineTrigger := PipelineTrigger{
		URL:       triggerURL,
		Token:     "TOKEN",
		Reference: "cloud",
		Variables: map[string]string{
			"A": "B",
			"C": "%%BIND_TO_C",
		},
	}

	data := getPipelineFormData(&pipelineTrigger, []string{"BIND_TO_C=BIND VALUE"})
	assert.NotNil(t, data)
	assert.Equal(t, "TOKEN", data.Get("token"))
	assert.Equal(t, "cloud", data.Get("ref"))
	assert.Equal(t, "B", data.Get("variables[A]"))
	assert.Equal(t, "BIND VALUE", data.Get("variables[C]"))
}

func TestPost(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("POST", triggerURL,
		func(req *http.Request) (*http.Response, error) {
			resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
				"value": "val",
			})
			return resp, err
		},
	)
	value, _ := post(triggerURL, url.Values{})
	val, ok := value["value"]
	if !ok {
		t.Errorf("Expected '{value:val}', got %s", value)
	}
	if val != "val" {
		t.Errorf("Expected 'val', got %s", val)
	}
}

func TestTriggerPipeline(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("POST", triggerURL,
		func(req *http.Request) (*http.Response, error) {
			req.ParseForm()

			assert.Equal(t, "TOKEN", req.FormValue("token"))
			assert.Equal(t, "cloud", req.FormValue("ref"))
			assert.Equal(t, "C VALUE", req.FormValue("variables[C]"))
			resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
				"web_url": pipelineURL,
			})
			return resp, err
		},
	)

	pipelineTrigger := PipelineTrigger{
		URL:       triggerURL,
		Token:     "TOKEN",
		Reference: "cloud",
		Variables: map[string]string{
			"C": "%%BIND_TO_C",
		},
	}
	value, err := TriggerPipeline(&pipelineTrigger, []string{"BIND_TO_C=C VALUE"})
	assert.Nil(t, err)
	assert.Equal(t, pipelineURL, value)
}

func TestTriggerPipelineInvalidToken(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("POST", triggerURL,
		func(req *http.Request) (*http.Response, error) {
			req.ParseForm()
			resp := httpmock.NewStringResponse(403, "Forbidden")
			return resp, nil
		},
	)

	pipelineTrigger := PipelineTrigger{
		URL:       triggerURL,
		Token:     "TOKENS",
		Reference: "cloud",
		Variables: map[string]string{
			"A":   "B",
			"Ref": "cloud",
			"C":   "%%BIND_TO_C",
		},
	}
	_, err := TriggerPipeline(&pipelineTrigger, []string{"BIND_TO_C=CC"})
	assert.NotNil(t, err)
	assert.Equal(t, "invalid request = 403,Forbidden", err.Error())
}
