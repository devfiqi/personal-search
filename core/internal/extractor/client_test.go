package extractor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClientExtractsPDFThroughWorker(t *testing.T) {
	python := requirePython(t)
	script := writeWorkerScript(t, `
import json
import sys
for line in sys.stdin:
    request = json.loads(line)
    print(json.dumps({"id": request["id"], "ok": True, "result": {"text": "worker text", "page_count": 3}}), flush=True)
`)
	client := New(Config{Executable: python, Arguments: []string{script}, Timeout: time.Second})
	defer client.Close()

	result, err := client.ExtractPDF(context.Background(), "/documents/example.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "worker text" || result.PageCount != 3 {
		t.Fatalf("ExtractPDF() = %+v", result)
	}
}

func TestClientReturnsRemoteErrorWithoutRetrying(t *testing.T) {
	python := requirePython(t)
	script := writeWorkerScript(t, `
import json
import sys
for line in sys.stdin:
    request = json.loads(line)
    print(json.dumps({"id": request["id"], "ok": False, "error": {"code": "encrypted_pdf", "message": "PDF requires a password"}}), flush=True)
`)
	client := New(Config{Executable: python, Arguments: []string{script}, Timeout: time.Second})
	defer client.Close()

	_, err := client.ExtractPDF(context.Background(), "/documents/encrypted.pdf")
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) {
		t.Fatalf("ExtractPDF() error = %v, want RemoteError", err)
	}
	if remoteError.Code != "encrypted_pdf" {
		t.Fatalf("RemoteError.Code = %q", remoteError.Code)
	}
}

func TestClientTimesOutAndStopsWorker(t *testing.T) {
	python := requirePython(t)
	script := writeWorkerScript(t, `
import sys
import time
for _ in sys.stdin:
    time.sleep(10)
`)
	client := New(Config{Executable: python, Arguments: []string{script}, Timeout: 25 * time.Millisecond})
	defer client.Close()

	_, err := client.ExtractPDF(context.Background(), "/documents/slow.pdf")
	if err == nil {
		t.Fatal("ExtractPDF() returned nil, want a timeout error")
	}
}

func TestClientRestartsWorkerAfterTransportFailure(t *testing.T) {
	python := requirePython(t)
	marker := filepath.Join(t.TempDir(), "first-attempt")
	script := writeWorkerScript(t, `
import json
import pathlib
import sys

marker = pathlib.Path(sys.argv[1])
for line in sys.stdin:
    request = json.loads(line)
    if not marker.exists():
        marker.touch()
        raise SystemExit(1)
    print(json.dumps({"id": request["id"], "ok": True, "result": {"text": "recovered", "page_count": 1}}), flush=True)
`)
	client := New(Config{
		Executable: python,
		Arguments:  []string{script, marker},
		Timeout:    time.Second,
	})
	defer client.Close()

	result, err := client.ExtractPDF(context.Background(), "/documents/retry.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" {
		t.Fatalf("ExtractPDF().Text = %q", result.Text)
	}
}

func requirePython(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	return python
}

func writeWorkerScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker.py")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
