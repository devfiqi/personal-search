package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/devfiqi/personal-search/core/internal/indexer"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

const Version = 1

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	Version int        `json:"version"`
	ID      string     `json:"id"`
	OK      bool       `json:"ok"`
	Result  any        `json:"result,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	socketPath string
	store      *storage.Store
	manager    *indexer.Manager
	searcher   Searcher
	listener   net.Listener
	closeOnce  sync.Once
}

type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]storage.SearchResult, error)
}

func New(socketPath string, store *storage.Store, manager *indexer.Manager) *Server {
	return NewWithSearcher(socketPath, store, manager, store)
}

func NewWithSearcher(socketPath string, store *storage.Store, manager *indexer.Manager, searcher Searcher) *Server {
	return &Server{socketPath: socketPath, store: store, manager: manager, searcher: searcher}
}

func (server *Server) Serve(ctx context.Context) error {
	if server.socketPath == "" {
		return errors.New("socket path is required")
	}
	if err := os.MkdirAll(filepath.Dir(server.socketPath), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}
	if err := removeStaleSocket(server.socketPath); err != nil {
		return err
	}

	listener, err := net.Listen("unix", server.socketPath)
	if err != nil {
		return fmt.Errorf("listen on local socket: %w", err)
	}
	server.listener = listener
	if err := os.Chmod(server.socketPath, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("secure local socket: %w", err)
	}
	defer server.Close()

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept local connection: %w", err)
		}
		go server.handleConnection(ctx, connection)
	}
}

func (server *Server) Close() {
	server.closeOnce.Do(func() {
		if server.listener != nil {
			_ = server.listener.Close()
		}
		_ = os.Remove(server.socketPath)
	})
}

func (server *Server) handleConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	decoder := json.NewDecoder(connection)
	encoder := json.NewEncoder(connection)
	for {
		var request Request
		if err := decoder.Decode(&request); err != nil {
			if !errors.Is(err, io.EOF) {
				_ = encoder.Encode(failure("", "invalid_request", "Request is not valid JSON"))
			}
			return
		}
		response := server.handle(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return
		}
	}
}

func (server *Server) handle(ctx context.Context, request Request) Response {
	if request.Version != Version {
		return failure(request.ID, "unsupported_version", "Unsupported local API version")
	}
	if request.ID == "" {
		return failure("", "invalid_request", "Request ID is required")
	}

	switch request.Method {
	case "state":
		folders, err := server.store.ListFolders(ctx)
		if err != nil {
			return internalFailure(request.ID, err)
		}
		failures, err := server.store.ListFailures(ctx, 100)
		if err != nil {
			return internalFailure(request.ID, err)
		}
		return success(request.ID, map[string]any{
			"indexing": server.manager.State(), "folders": folders, "failure_count": len(failures),
		})
	case "folders":
		folders, err := server.store.ListFolders(ctx)
		if err != nil {
			return internalFailure(request.ID, err)
		}
		return success(request.ID, folders)
	case "add_folder":
		var params struct {
			Path string `json:"path"`
		}
		if err := decodeParams(request.Params, &params); err != nil || params.Path == "" {
			return failure(request.ID, "invalid_request", "A folder path is required")
		}
		path, err := validateFolder(params.Path)
		if err != nil {
			return failure(request.ID, "invalid_folder", err.Error())
		}
		if _, err := server.store.RegisterFolder(ctx, path); err != nil {
			return internalFailure(request.ID, err)
		}
		server.manager.ReloadFolders()
		return success(request.ID, map[string]string{"path": path})
	case "remove_folder":
		var params struct {
			Path string `json:"path"`
		}
		if err := decodeParams(request.Params, &params); err != nil || params.Path == "" {
			return failure(request.ID, "invalid_request", "A folder path is required")
		}
		path, err := filepath.Abs(params.Path)
		if err != nil {
			return failure(request.ID, "invalid_folder", "Folder path is invalid")
		}
		removed, err := server.store.RemoveFolder(ctx, filepath.Clean(path))
		if err != nil {
			return internalFailure(request.ID, err)
		}
		server.manager.ReloadFolders()
		return success(request.ID, map[string]bool{"removed": removed})
	case "search":
		var params struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := decodeParams(request.Params, &params); err != nil {
			return failure(request.ID, "invalid_request", "Search parameters are invalid")
		}
		results, err := server.searcher.Search(ctx, params.Query, params.Limit)
		if err != nil {
			return internalFailure(request.ID, err)
		}
		return success(request.ID, results)
	case "failures":
		failures, err := server.store.ListFailures(ctx, 100)
		if err != nil {
			return internalFailure(request.ID, err)
		}
		return success(request.ID, failures)
	case "pause":
		server.manager.SetPaused(true)
		return success(request.ID, server.manager.State())
	case "resume":
		server.manager.SetPaused(false)
		return success(request.ID, server.manager.State())
	case "reset":
		server.manager.SetPaused(true)
		if err := server.store.Reset(ctx); err != nil {
			return internalFailure(request.ID, err)
		}
		server.manager.ReloadFolders()
		server.manager.SetPaused(false)
		return success(request.ID, map[string]string{"status": "reset"})
	default:
		return failure(request.ID, "unsupported_method", "Unsupported local API method")
	}
}

func validateFolder(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("Folder path is invalid")
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return "", errors.New("Folder cannot be accessed")
	}
	if !info.IsDir() {
		return "", errors.New("Selected path is not a folder")
	}
	return absolute, nil
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect local socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("local API path exists and is not a socket")
	}
	if !ownsSocket(info) {
		return errors.New("local search service is already running")
	}

	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	connection, err := dialer.Dial("unix", path)
	if err == nil {
		_ = connection.Close()
		return errors.New("local search service is already running")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale local socket: %w", err)
	}
	return nil
}

func ownsSocket(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == uint32(os.Getuid())
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("missing parameters")
	}
	return json.Unmarshal(raw, target)
}

func success(id string, result any) Response {
	return Response{Version: Version, ID: id, OK: true, Result: result}
}

func failure(id string, code string, message string) Response {
	return Response{Version: Version, ID: id, OK: false, Error: &ErrorBody{Code: code, Message: message}}
}

func internalFailure(id string, err error) Response {
	return failure(id, "internal_error", err.Error())
}
