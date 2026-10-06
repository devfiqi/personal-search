package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunInitializesDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "search.db")
	var output bytes.Buffer
	var errorsOutput bytes.Buffer

	if err := Run([]string{"init", "--db", databasePath}, &output, &errorsOutput); err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}

	if got, want := output.String(), "{\"database\":\""+databasePath+"\",\"status\":\"ready\"}\n"; got != want {
		t.Fatalf("Run() output = %q, want %q", got, want)
	}
}

func TestCancelWhenParentExitsKeepsALivingParent(t *testing.T) {
	ctx, cancel := cancelWhenParentExits(context.Background())
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("service stopped while its parent was still running")
	case <-time.After(500 * time.Millisecond):
	}
	if os.Getppid() <= 0 {
		t.Fatal("parent process id was unavailable")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() returned nil, want an error")
	}
}
