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
