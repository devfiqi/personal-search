package extractor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const defaultTimeout = 60 * time.Second

type Config struct {
	Executable  string
	Arguments   []string
	Environment []string
	Timeout     time.Duration
}

type Client struct {
	mutex       sync.Mutex
	config      Config
	command     *exec.Cmd
	input       io.WriteCloser
	encoder     *json.Encoder
	decoder     *json.Decoder
	nextRequest uint64
}

type PDFResult struct {
	Text      string `json:"text"`
	PageCount int    `json:"page_count"`
}

type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (failure *RemoteError) Error() string {
	return failure.Code + ": " + failure.Message
}

type request struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Path      string `json:"path,omitempty"`
}

type response struct {
	ID     string       `json:"id"`
	OK     bool         `json:"ok"`
	Result PDFResult    `json:"result"`
	Error  *RemoteError `json:"error"`
}

type callResult struct {
	response response
	err      error
}

func New(config Config) *Client {
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	return &Client{config: config}
}

func NewPython(executable string, modulePath string, timeout time.Duration) *Client {
	environment := []string{}
	if modulePath != "" {
		pythonPath := modulePath
		if existing := os.Getenv("PYTHONPATH"); existing != "" {
			pythonPath += string(filepath.ListSeparator) + existing
		}
		environment = append(environment, "PYTHONPATH="+pythonPath)
	}

	return New(Config{
		Executable:  executable,
		Arguments:   []string{"-m", "personal_search_extractor"},
		Environment: environment,
		Timeout:     timeout,
	})
}

func NewExecutable(executable string, timeout time.Duration) *Client {
	return New(Config{Executable: executable, Timeout: timeout})
}

func (client *Client) ExtractPDF(ctx context.Context, path string) (PDFResult, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	var lastError error
	for attempt := 0; attempt < 2; attempt++ {
		result, retry, err := client.extractPDFLocked(ctx, path)
		if err == nil {
			return result, nil
		}
		lastError = err
		if !retry {
			return PDFResult{}, err
		}
	}
	return PDFResult{}, fmt.Errorf("PDF worker failed after retry: %w", lastError)
}

func (client *Client) Close() error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return client.stopLocked(false)
}

func (client *Client) extractPDFLocked(ctx context.Context, path string) (PDFResult, bool, error) {
	if err := client.startLocked(); err != nil {
		return PDFResult{}, true, err
	}

	client.nextRequest++
	requestID := strconv.FormatUint(client.nextRequest, 10)
	message := request{ID: requestID, Operation: "extract_pdf", Path: path}
	encoder := client.encoder
	decoder := client.decoder

	completed := make(chan callResult, 1)
	go func() {
		if err := encoder.Encode(message); err != nil {
			completed <- callResult{err: err}
			return
		}
		var reply response
		if err := decoder.Decode(&reply); err != nil {
			completed <- callResult{err: err}
			return
		}
		completed <- callResult{response: reply}
	}()

	timer := time.NewTimer(client.config.Timeout)
	defer timer.Stop()

	select {
	case result := <-completed:
		if result.err != nil {
			_ = client.stopLocked(true)
			return PDFResult{}, true, fmt.Errorf("communicate with PDF worker: %w", result.err)
		}
		if result.response.ID != requestID {
			_ = client.stopLocked(true)
			return PDFResult{}, true, fmt.Errorf("PDF worker returned mismatched response ID")
		}
		if !result.response.OK {
			if result.response.Error == nil {
				return PDFResult{}, false, fmt.Errorf("PDF worker returned an unspecified error")
			}
			return PDFResult{}, false, result.response.Error
		}
		return result.response.Result, false, nil
	case <-ctx.Done():
		_ = client.stopLocked(true)
		drainCall(completed)
		return PDFResult{}, false, ctx.Err()
	case <-timer.C:
		_ = client.stopLocked(true)
		drainCall(completed)
		return PDFResult{}, true, fmt.Errorf("PDF extraction timed out")
	}
}

func drainCall(completed <-chan callResult) {
	select {
	case <-completed:
	case <-time.After(time.Second):
	}
}

func (client *Client) startLocked() error {
	if client.command != nil {
		return nil
	}
	if client.config.Executable == "" {
		return errors.New("PDF worker executable is required")
	}

	command := exec.Command(client.config.Executable, client.config.Arguments...)
	command.Env = append(os.Environ(), client.config.Environment...)
	command.Stderr = io.Discard

	input, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("open PDF worker input: %w", err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return fmt.Errorf("open PDF worker output: %w", err)
	}
	if err := command.Start(); err != nil {
		input.Close()
		return fmt.Errorf("start PDF worker: %w", err)
	}

	client.command = command
	client.input = input
	client.encoder = json.NewEncoder(input)
	client.decoder = json.NewDecoder(output)
	return nil
}

func (client *Client) stopLocked(force bool) error {
	if client.command == nil {
		return nil
	}

	command := client.command
	input := client.input
	client.command = nil
	client.input = nil
	client.encoder = nil
	client.decoder = nil

	if force && command.Process != nil {
		_ = command.Process.Kill()
	} else if input != nil {
		_ = input.Close()
	}

	waited := make(chan error, 1)
	go func() {
		waited <- command.Wait()
	}()

	select {
	case err := <-waited:
		if err != nil && !force {
			return fmt.Errorf("stop PDF worker: %w", err)
		}
		return nil
	case <-time.After(time.Second):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-waited
		return nil
	}
}
