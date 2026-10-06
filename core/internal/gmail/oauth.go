// Package gmail contains the narrowly scoped Gmail OAuth connection flow.
// Mail content is never written by this package and OAuth refresh tokens are
// returned only to the native app for Keychain storage.
package gmail

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const ReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"

const (
	defaultAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenEndpoint         = "https://oauth2.googleapis.com/token"
	defaultProfileEndpoint       = "https://gmail.googleapis.com/gmail/v1/users/me/profile"
	defaultMessagesEndpoint      = "https://gmail.googleapis.com/gmail/v1/users/me/messages"
)

type Account struct {
	Email        string `json:"email"`
	ClientID     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
}

type Client struct {
	HTTPClient            *http.Client
	AuthorizationEndpoint string
	TokenEndpoint         string
	ProfileEndpoint       string
	MessagesEndpoint      string
	OpenURL               func(string) error
	Listen                func(network string, address string) (net.Listener, error)
	Random                io.Reader
}

func NewClient() *Client {
	return &Client{
		HTTPClient:            &http.Client{Timeout: 20 * time.Second},
		AuthorizationEndpoint: defaultAuthorizationEndpoint,
		TokenEndpoint:         defaultTokenEndpoint,
		ProfileEndpoint:       defaultProfileEndpoint,
		MessagesEndpoint:      defaultMessagesEndpoint,
		OpenURL: func(value string) error {
			return exec.Command("open", value).Start()
		},
		Listen: net.Listen,
		Random: rand.Reader,
	}
}

func (client *Client) Connect(ctx context.Context, clientID string) (Account, error) {
	if !strings.HasSuffix(clientID, ".apps.googleusercontent.com") {
		return Account{}, errors.New("a Google Desktop OAuth client ID is required")
	}
	listener, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Account{}, fmt.Errorf("start Gmail authorization listener: %w", err)
	}
	defer listener.Close()

	state, err := client.randomURLValue(32)
	if err != nil {
		return Account{}, err
	}
	verifier, err := client.randomURLValue(64)
	if err != nil {
		return Account{}, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	callbackURL := "http://" + listener.Addr().String() + "/oauth2/callback"
	callback := make(chan callbackResult, 1)
	server := &http.Server{Handler: client.callbackHandler(state, callback)}
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())

	authorizationURL, err := url.Parse(client.AuthorizationEndpoint)
	if err != nil {
		return Account{}, fmt.Errorf("build Gmail authorization URL: %w", err)
	}
	parameters := authorizationURL.Query()
	parameters.Set("client_id", clientID)
	parameters.Set("redirect_uri", callbackURL)
	parameters.Set("response_type", "code")
	parameters.Set("scope", ReadonlyScope)
	parameters.Set("access_type", "offline")
	parameters.Set("prompt", "consent")
	parameters.Set("state", state)
	parameters.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	parameters.Set("code_challenge_method", "S256")
	authorizationURL.RawQuery = parameters.Encode()
	if err := client.OpenURL(authorizationURL.String()); err != nil {
		return Account{}, fmt.Errorf("open Gmail authorization: %w", err)
	}

	select {
	case result := <-callback:
		if result.err != nil {
			return Account{}, result.err
		}
		token, err := client.exchange(ctx, clientID, callbackURL, verifier, result.code)
		if err != nil {
			return Account{}, err
		}
		if token.RefreshToken == "" {
			return Account{}, errors.New("Gmail did not return a refresh token; remove this app from Google Account permissions and try again")
		}
		email, err := client.profile(ctx, token.AccessToken)
		if err != nil {
			return Account{}, err
		}
		return Account{Email: email, ClientID: clientID, RefreshToken: token.RefreshToken}, nil
	case <-ctx.Done():
		return Account{}, ctx.Err()
	}
}

type callbackResult struct {
	code string
	err  error
}

func (client *Client) callbackHandler(expectedState string, callback chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if request.URL.Path != "/oauth2/callback" || request.URL.Query().Get("state") != expectedState {
			http.Error(writer, "Authorization could not be verified. You can close this window.", http.StatusBadRequest)
			return
		}
		if message := request.URL.Query().Get("error"); message != "" {
			select {
			case callback <- callbackResult{err: fmt.Errorf("Gmail authorization was not granted: %s", message)}:
			default:
			}
			_, _ = io.WriteString(writer, "<p>Gmail access was not granted. You can close this window.</p>")
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			http.Error(writer, "Authorization did not include a code. You can close this window.", http.StatusBadRequest)
			return
		}
		select {
		case callback <- callbackResult{code: code}:
		default:
		}
		_, _ = io.WriteString(writer, "<p>Gmail is connected. You can close this window and return to Personal Search.</p>")
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (client *Client) exchange(ctx context.Context, clientID string, redirectURI string, verifier string, code string) (tokenResponse, error) {
	values := url.Values{
		"client_id": {clientID}, "code": {code}, "code_verifier": {verifier},
		"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("create Gmail token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("exchange Gmail authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("exchange Gmail authorization code: received HTTP %d", response.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return tokenResponse{}, fmt.Errorf("decode Gmail token response: %w", err)
	}
	if token.AccessToken == "" {
		return tokenResponse{}, errors.New("Gmail token response did not include an access token")
	}
	return token, nil
}

func (client *Client) profile(ctx context.Context, accessToken string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.ProfileEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create Gmail profile request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.httpClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("read Gmail profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("read Gmail profile: received HTTP %d", response.StatusCode)
	}
	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return "", fmt.Errorf("decode Gmail profile: %w", err)
	}
	if profile.EmailAddress == "" {
		return "", errors.New("Gmail profile did not include an email address")
	}
	return profile.EmailAddress, nil
}

func (client *Client) randomURLValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(client.Random, value); err != nil {
		return "", fmt.Errorf("generate Gmail authorization value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (client *Client) httpClient() *http.Client {
	if client.HTTPClient != nil {
		return client.HTTPClient
	}
	return http.DefaultClient
}
