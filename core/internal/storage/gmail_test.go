package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGmailAccountsCanBeRegisteredAndRemoved(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	account := GmailAccount{Email: "person@example.com", ClientID: "client.apps.googleusercontent.com"}
	if err := store.RegisterGmailAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.ListGmailAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0] != account {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	removed, err := store.RemoveGmailAccount(ctx, account.Email)
	if err != nil || !removed {
		t.Fatalf("RemoveGmailAccount() = %t, %v", removed, err)
	}
}

func TestResetClearsGmailAccounts(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.RegisterGmailAccount(ctx, GmailAccount{Email: "person@example.com", ClientID: "client.apps.googleusercontent.com"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.ListGmailAccounts(ctx)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("accounts after reset = %+v, %v", accounts, err)
	}
}

func TestStoreGmailPageMakesMailSearchable(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.RegisterGmailAccount(ctx, GmailAccount{Email: "person@example.com", ClientID: "client.apps.googleusercontent.com"}); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreGmailPage(ctx, "person@example.com", []EmailMessage{{GmailID: "abc", ThreadID: "thread", Subject: "Project roadmap", Sender: "sender@example.com", ReceivedAtNS: 1, Body: "Discuss the local search launch."}}, "next"); err != nil {
		t.Fatal(err)
	}
	state, err := store.GmailSyncState(ctx, "person@example.com")
	if err != nil || state.NextPageToken != "next" || state.SyncComplete {
		t.Fatalf("state = %+v, %v", state, err)
	}
	results, err := store.Search(ctx, "roadmap", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Source != "email" || results[0].Name != "Project roadmap" {
		t.Fatalf("results = %+v", results)
	}
	if err := store.StoreGmailPage(ctx, "person@example.com", nil, ""); err != nil {
		t.Fatal(err)
	}
	state, err = store.GmailSyncState(ctx, "person@example.com")
	if err != nil || !state.SyncComplete {
		t.Fatalf("complete state = %+v, %v", state, err)
	}
}
