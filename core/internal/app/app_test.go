package app

import (
	"bytes"
	"path/filepath"
	"testing"
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

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() returned nil, want an error")
	}
}
