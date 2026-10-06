package gmail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestConnectUsesPKCEAndReadonlyScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("code_verifier") == "" || request.Form.Get("client_id") == "" {
				t.Fatal("missing PKCE token exchange values")
			}
			_, _ = io.WriteString(writer, `{"access_token":"access","refresh_token":"refresh"}`)
		case "/profile":
			if request.Header.Get("Authorization") != "Bearer access" {
				t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(writer, `{"emailAddress":"person@example.com"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.AuthorizationEndpoint = server.URL + "/authorize"
	client.TokenEndpoint = server.URL + "/token"
	client.ProfileEndpoint = server.URL + "/profile"
	client.OpenURL = func(value string) error {
		parsed, err := url.Parse(value)
		if err != nil {
			return err
		}
		if parsed.Query().Get("scope") != ReadonlyScope || parsed.Query().Get("code_challenge_method") != "S256" {
			t.Fatalf("authorization URL = %s", value)
		}
		callback := parsed.Query().Get("redirect_uri")
		state := parsed.Query().Get("state")
		go func() { _, _ = http.Get(callback + "?state=" + url.QueryEscape(state) + "&code=authorization-code") }()
		return nil
	}

	account, err := client.Connect(context.Background(), "client.apps.googleusercontent.com")
	if err != nil {
		t.Fatal(err)
	}
	if account.Email != "person@example.com" || account.RefreshToken != "refresh" {
		t.Fatalf("account = %+v", account)
	}
}

func TestConnectRejectsNonGoogleClientID(t *testing.T) {
	_, err := NewClient().Connect(context.Background(), "not-a-client-id")
	if err == nil || !strings.Contains(err.Error(), "Desktop OAuth client ID") {
		t.Fatalf("Connect() error = %v", err)
	}
}
