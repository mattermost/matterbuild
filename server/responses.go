// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"encoding/json"
)

type MMSlashResponse struct {
	ResponseType string        `json:"response_type"`
	Text         string        `json:"text"`
	GotoLocation string        `json:"goto_location"`
	Username     string        `json:"username"`
	Attachments  *[]Attachment `json:"attachments"`
	IconURL      string        `json:"icon_url"`
}

type Attachment struct {
	ID         int64              `json:"id"`
	Fallback   string             `json:"fallback"`
	Color      string             `json:"color"`
	Pretext    string             `json:"pretext"`
	AuthorName string             `json:"author_name"`
	AuthorLink string             `json:"author_link"`
	AuthorIcon string             `json:"author_icon"`
	Title      string             `json:"title"`
	TitleLink  string             `json:"title_link"`
	Text       string             `json:"text"`
	Fields     []*AttachmentField `json:"fields"`
	ImageURL   string             `json:"image_url"`
	ThumbURL   string             `json:"thumb_url"`
	Footer     string             `json:"footer"`
	FooterIcon string             `json:"footer_icon"`
	Timestamp  interface{}        `json:"ts"` // This is either a string or an int64
}

type AttachmentField struct {
	Title string      `json:"title"`
	Value interface{} `json:"value"`
	Short bool        `json:"short"`
}

func GenerateStandardSlashResponse(text string, respType string) string {
	response := MMSlashResponse{
		ResponseType: respType,
		Text:         text,
		GotoLocation: "",
		Username:     "Matterbuild",
		IconURL:      "https://mattermost.com/wp-content/uploads/2022/02/icon.png",
	}

	b, err := json.Marshal(response)
	if err != nil {
		LogError("Unable to marshal response")
		return ""
	}
	return string(b)
}

func GenerateEnrichedSlashResponse(title, text, color, respType string) []byte {
	msgAttachment := &[]Attachment{{
		Fallback:   text,
		Color:      color,
		Text:       text,
		Title:      title,
		AuthorName: "Matterbuild",
		AuthorIcon: "https://mattermost.com/wp-content/uploads/2022/02/icon.png",
	}}

	response := MMSlashResponse{
		ResponseType: respType,
		Text:         "",
		Attachments:  msgAttachment,
		GotoLocation: "",
		Username:     "Matterbuild",
		IconURL:      "https://mattermost.com/wp-content/uploads/2022/02/icon.png",
	}

	b, err := json.Marshal(response)
	if err != nil {
		LogError("Unable to marshal response")
		return nil
	}

	return b
}
