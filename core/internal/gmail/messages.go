package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const pageSize = 50

type Message struct {
	ID           string
	ThreadID     string
	Subject      string
	Sender       string
	Recipients   string
	ReceivedAtNS int64
	Body         string
}

type Page struct {
	Messages      []Message
	NextPageToken string
}

// FetchPage refreshes access in memory and downloads one bounded page of mail.
// The caller owns the refresh token and decides how to persist indexed content.
func (client *Client) FetchPage(ctx context.Context, clientID string, refreshToken string, pageToken string) (Page, error) {
	if clientID == "" || refreshToken == "" {
		return Page{}, errors.New("Gmail credentials are required")
	}
	accessToken, err := client.refresh(ctx, clientID, refreshToken)
	if err != nil {
		return Page{}, err
	}
	return client.messages(ctx, accessToken, pageToken)
}

func (client *Client) refresh(ctx context.Context, clientID string, refreshToken string) (string, error) {
	values := url.Values{
		"client_id": {clientID}, "refresh_token": {refreshToken}, "grant_type": {"refresh_token"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create Gmail refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("refresh Gmail access: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("refresh Gmail access: received HTTP %d", response.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return "", fmt.Errorf("decode Gmail refresh response: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("Gmail refresh response did not include an access token")
	}
	return token.AccessToken, nil
}

func (client *Client) messages(ctx context.Context, accessToken string, pageToken string) (Page, error) {
	endpoint, err := url.Parse(client.MessagesEndpoint)
	if err != nil {
		return Page{}, fmt.Errorf("build Gmail messages request: %w", err)
	}
	query := endpoint.Query()
	query.Set("maxResults", strconv.Itoa(pageSize))
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Page{}, fmt.Errorf("create Gmail messages request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.httpClient().Do(request)
	if err != nil {
		return Page{}, fmt.Errorf("list Gmail messages: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Page{}, fmt.Errorf("list Gmail messages: received HTTP %d", response.StatusCode)
	}
	var listed struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&listed); err != nil {
		return Page{}, fmt.Errorf("decode Gmail messages: %w", err)
	}
	page := Page{Messages: make([]Message, 0, len(listed.Messages)), NextPageToken: listed.NextPageToken}
	for _, listedMessage := range listed.Messages {
		message, err := client.message(ctx, accessToken, listedMessage.ID)
		if err != nil {
			return Page{}, err
		}
		page.Messages = append(page.Messages, message)
	}
	return page, nil
}

func (client *Client) message(ctx context.Context, accessToken string, id string) (Message, error) {
	endpoint := strings.TrimRight(client.MessagesEndpoint, "/") + "/" + url.PathEscape(id)
	query := url.Values{"format": {"full"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return Message{}, fmt.Errorf("create Gmail message request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.httpClient().Do(request)
	if err != nil {
		return Message{}, fmt.Errorf("read Gmail message: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Message{}, fmt.Errorf("read Gmail message: received HTTP %d", response.StatusCode)
	}
	var raw gmailMessage
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&raw); err != nil {
		return Message{}, fmt.Errorf("decode Gmail message: %w", err)
	}
	if raw.ID == "" {
		return Message{}, errors.New("Gmail message did not include an ID")
	}
	return Message{
		ID: raw.ID, ThreadID: raw.ThreadID, Subject: raw.header("Subject"), Sender: raw.header("From"),
		Recipients: joinAddresses(raw.header("To"), raw.header("Cc")), ReceivedAtNS: raw.receivedAtNS(), Body: raw.Payload.text(),
	}, nil
}

type gmailMessage struct {
	ID           string         `json:"id"`
	ThreadID     string         `json:"threadId"`
	InternalDate string         `json:"internalDate"`
	Payload      messagePayload `json:"payload"`
}

func (message gmailMessage) header(name string) string {
	for _, header := range message.Payload.Headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func (message gmailMessage) receivedAtNS() int64 {
	milliseconds, err := strconv.ParseInt(message.InternalDate, 10, 64)
	if err == nil && milliseconds > 0 {
		return milliseconds * int64(time.Millisecond)
	}
	if value, err := time.Parse(time.RFC1123Z, message.header("Date")); err == nil {
		return value.UnixNano()
	}
	return 0
}

type messagePayload struct {
	MimeType string `json:"mimeType"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []messagePayload `json:"parts"`
}

func (payload messagePayload) text() string {
	if strings.EqualFold(payload.MimeType, "text/plain") && payload.Body.Data != "" {
		return decodeBody(payload.Body.Data)
	}
	for _, part := range payload.Parts {
		if text := part.text(); text != "" {
			return text
		}
	}
	if strings.EqualFold(payload.MimeType, "text/html") && payload.Body.Data != "" {
		return stripHTML(decodeBody(payload.Body.Data))
	}
	return ""
}

func decodeBody(value string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func joinAddresses(values ...string) string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			output = append(output, value)
		}
	}
	return strings.Join(output, ", ")
}

func stripHTML(value string) string {
	var output strings.Builder
	inTag := false
	for _, character := range value {
		switch character {
		case '<':
			inTag = true
		case '>':
			inTag = false
			output.WriteByte(' ')
		default:
			if !inTag {
				output.WriteRune(character)
			}
		}
	}
	return strings.Join(strings.Fields(output.String()), " ")
}
