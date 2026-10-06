package storage

import (
	"context"
	"fmt"
)

type GmailAccount struct {
	Email    string `json:"email"`
	ClientID string `json:"client_id"`
}

func (store *Store) RegisterGmailAccount(ctx context.Context, account GmailAccount) error {
	if account.Email == "" || account.ClientID == "" {
		return fmt.Errorf("Gmail account email and client ID are required")
	}
	if _, err := store.database.ExecContext(ctx, `
INSERT INTO gmail_accounts(email, client_id) VALUES (?, ?)
ON CONFLICT(email) DO UPDATE SET client_id = excluded.client_id, connected_at = CURRENT_TIMESTAMP`, account.Email, account.ClientID); err != nil {
		return fmt.Errorf("register Gmail account: %w", err)
	}
	return nil
}

func (store *Store) ListGmailAccounts(ctx context.Context) ([]GmailAccount, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT email, client_id FROM gmail_accounts ORDER BY email")
	if err != nil {
		return nil, fmt.Errorf("list Gmail accounts: %w", err)
	}
	defer rows.Close()
	accounts := []GmailAccount{}
	for rows.Next() {
		var account GmailAccount
		if err := rows.Scan(&account.Email, &account.ClientID); err != nil {
			return nil, fmt.Errorf("read Gmail account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Gmail accounts: %w", err)
	}
	return accounts, nil
}

func (store *Store) RemoveGmailAccount(ctx context.Context, email string) (bool, error) {
	result, err := store.database.ExecContext(ctx, "DELETE FROM gmail_accounts WHERE email = ?", email)
	if err != nil {
		return false, fmt.Errorf("remove Gmail account: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read removed Gmail account: %w", err)
	}
	return count > 0, nil
}
