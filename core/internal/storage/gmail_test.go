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
