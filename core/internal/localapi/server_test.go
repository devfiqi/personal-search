package localapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devfiqi/personal-search/core/internal/gmail"
	"github.com/devfiqi/personal-search/core/internal/indexer"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

func TestServerSupportsFolderSearchControlsAndReset(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "personal-search-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	documents := filepath.Join(root, "documents")
	if err := os.Mkdir(documents, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(documents, "notes.txt"), []byte("violet handbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(root, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := indexer.NewManager(indexer.New(store, nil), store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	socket := filepath.Join(root, "search.sock")
	server := New(socket, store, manager)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(ctx) }()
	waitForSocket(t, socket, serverDone)

	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)

	request(t, connection, reader, Request{Version: 1, ID: "1", Method: "add_folder", Params: raw(map[string]any{"path": documents})}, true)
	deadline := time.Now().Add(3 * time.Second)
	for {
		response := request(t, connection, reader, Request{Version: 1, ID: "2", Method: "search", Params: raw(map[string]any{"query": "violet", "limit": 10})}, true)
		results := decodeResult[[]storage.SearchResult](t, response)
		if len(results) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("document was not indexed")
		}
		time.Sleep(25 * time.Millisecond)
	}

	paused := request(t, connection, reader, Request{Version: 1, ID: "3", Method: "pause"}, true)
	if !decodeResult[indexer.ManagerState](t, paused).Paused {
		t.Fatal("manager did not pause")
	}
	request(t, connection, reader, Request{Version: 1, ID: "4", Method: "resume"}, true)
	request(t, connection, reader, Request{Version: 1, ID: "5", Method: "reset"}, true)
	foldersResponse := request(t, connection, reader, Request{Version: 1, ID: "6", Method: "folders"}, true)
	if folders := decodeResult[[]storage.Folder](t, foldersResponse); len(folders) != 0 {
		t.Fatalf("folders after reset = %+v", folders)
	}

	cancel()
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestServerRejectsAnAlreadyRunningSocket(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "personal-search-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	store, err := storage.Open(filepath.Join(root, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := indexer.NewManager(indexer.New(store, nil), store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	socket := filepath.Join(root, "search.sock")
	server := New(socket, store, manager)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(ctx) }()
	waitForSocket(t, socket, serverDone)

	secondDone := make(chan error, 1)
	go func() { secondDone <- New(socket, store, manager).Serve(ctx) }()
	select {
	case err := <-secondDone:
		if err == nil || !strings.Contains(err.Error(), "already running") {
			t.Fatalf("second serve error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second serve did not stop")
	}

	cancel()
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestServerRejectsWrongVersion(t *testing.T) {
	response := failure("request", "unsupported_version", "Unsupported local API version")
	if response.OK || response.Error.Code != "unsupported_version" {
		t.Fatalf("failure response = %+v", response)
	}
}

func TestServerConnectsAndRegistersGmailAccount(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := indexer.NewManager(indexer.New(store, nil), store)
	server := New("", store, manager)
	server.SetGmailConnector(fakeGmailConnector{})

	connected := server.handle(context.Background(), Request{Version: 1, ID: "connect", Method: "gmail_connect", Params: raw(map[string]string{"client_id": "client.apps.googleusercontent.com"})})
	account := decodeResult[gmail.Account](t, connected)
	if account.RefreshToken != "refresh-token" {
		t.Fatalf("account = %+v", account)
	}
	registered := server.handle(context.Background(), Request{Version: 1, ID: "add", Method: "gmail_add_account", Params: raw(storage.GmailAccount{Email: account.Email, ClientID: account.ClientID})})
	if !registered.OK {
		t.Fatalf("register response = %+v", registered)
	}
	accounts := server.handle(context.Background(), Request{Version: 1, ID: "list", Method: "gmail_accounts", Params: raw(map[string]string{})})
	listed := decodeResult[[]storage.GmailAccount](t, accounts)
	if len(listed) != 1 || listed[0].Email != account.Email {
		t.Fatalf("accounts = %+v", listed)
	}
}

type fakeGmailConnector struct{}

func (fakeGmailConnector) Connect(context.Context, string) (gmail.Account, error) {
	return gmail.Account{Email: "person@example.com", ClientID: "client.apps.googleusercontent.com", RefreshToken: "refresh-token"}, nil
}

func waitForSocket(t *testing.T, path string, serverDone <-chan error) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-serverDone:
			t.Fatalf("local server stopped before listening: %v", err)
		default:
		}
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("local socket did not appear")
}

func request(t *testing.T, connection net.Conn, reader *bufio.Reader, value Request, wantOK bool) Response {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if _, err := connection.Write(encoded); err != nil {
		t.Fatal(err)
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatal(err)
	}
	if response.OK != wantOK {
		t.Fatalf("response = %+v", response)
	}
	return response
}

func raw(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func decodeResult[T any](t *testing.T, response Response) T {
	t.Helper()
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
