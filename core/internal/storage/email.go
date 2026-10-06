package storage

import (
	"context"
	"fmt"
)

type EmailMessage struct {
	GmailID      string
	ThreadID     string
	Subject      string
	Sender       string
	Recipients   string
	ReceivedAtNS int64
	Body         string
}

type GmailSyncState struct {
	Email         string
	ClientID      string
	NextPageToken string
	SyncComplete  bool
}

func (store *Store) GmailSyncState(ctx context.Context, email string) (GmailSyncState, error) {
	var state GmailSyncState
	var complete int
	err := store.database.QueryRowContext(ctx, `
SELECT email, client_id, next_page_token, sync_complete FROM gmail_accounts WHERE email = ?`, email).Scan(
		&state.Email, &state.ClientID, &state.NextPageToken, &complete,
	)
	if err != nil {
		return GmailSyncState{}, fmt.Errorf("read Gmail sync state: %w", err)
	}
	state.SyncComplete = complete != 0
	return state, nil
}

func (store *Store) StoreGmailPage(ctx context.Context, email string, messages []EmailMessage, nextPageToken string) error {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Gmail page update: %w", err)
	}
	defer transaction.Rollback()
	for _, message := range messages {
		if message.GmailID == "" {
			return fmt.Errorf("Gmail message ID is required")
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO email_messages(account_email, gmail_id, thread_id, subject, sender, recipients, received_at_ns, body)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account_email, gmail_id) DO UPDATE SET
    thread_id = excluded.thread_id, subject = excluded.subject, sender = excluded.sender,
    recipients = excluded.recipients, received_at_ns = excluded.received_at_ns,
    body = excluded.body, indexed_at = CURRENT_TIMESTAMP`,
			email, message.GmailID, message.ThreadID, message.Subject, message.Sender, message.Recipients, message.ReceivedAtNS, message.Body,
		); err != nil {
			return fmt.Errorf("store Gmail message: %w", err)
		}
	}
	complete := 0
	if nextPageToken == "" {
		complete = 1
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE gmail_accounts SET next_page_token = ?, sync_complete = ? WHERE email = ?`, nextPageToken, complete, email)
	if err != nil {
		return fmt.Errorf("update Gmail sync state: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		if err != nil {
			return fmt.Errorf("read Gmail sync state update: %w", err)
		}
		return fmt.Errorf("Gmail account is not connected")
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit Gmail page update: %w", err)
	}
	return nil
}
