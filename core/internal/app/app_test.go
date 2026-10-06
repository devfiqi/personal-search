package app

import (
	"bytes"
	"testing"
)

func TestRunWritesCoreName(t *testing.T) {
	var output bytes.Buffer

	if err := Run(&output); err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}

	if got, want := output.String(), Name+"\n"; got != want {
		t.Fatalf("Run() output = %q, want %q", got, want)
	}
}
