package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/devfiqi/personal-search/core/internal/extractor"
	"github.com/devfiqi/personal-search/core/internal/indexer"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

const Name = "personal-search-core"

func Run(args []string, output io.Writer, errorsOutput io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected a command: init, index, or search")
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], output, errorsOutput)
	case "index":
		return runIndex(args[1:], output, errorsOutput)
	case "search":
		return runSearch(args[1:], output, errorsOutput)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string, output io.Writer, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	databasePath := flags.String("db", "", "path to the SQLite database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *databasePath == "" {
		return errors.New("init requires --db")
	}

	store, err := storage.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	return writeJSON(output, map[string]any{"status": "ready", "database": *databasePath})
}

func runIndex(args []string, output io.Writer, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	databasePath := flags.String("db", "", "path to the SQLite database")
	folderPath := flags.String("folder", "", "folder to index")
	pythonExecutable := flags.String("python", defaultValue("PERSONAL_SEARCH_PYTHON", "python3"), "Python worker executable")
	extractorSource := flags.String("extractor-src", defaultValue("PERSONAL_SEARCH_EXTRACTOR_SRC", "extractor/src"), "Python extractor source directory")
	extractionTimeout := flags.Duration("extract-timeout", 60*time.Second, "PDF extraction timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *databasePath == "" || *folderPath == "" {
		return errors.New("index requires --db and --folder")
	}

	store, err := storage.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	absoluteExtractorSource, err := filepath.Abs(*extractorSource)
	if err != nil {
		return fmt.Errorf("resolve extractor source: %w", err)
	}
	worker := extractor.NewPython(*pythonExecutable, absoluteExtractorSource, *extractionTimeout)
	defer worker.Close()

	report, err := indexer.New(store, worker).IndexFolder(context.Background(), *folderPath)
	if err != nil {
		return err
	}

	return writeJSON(output, report)
}

func runSearch(args []string, output io.Writer, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	databasePath := flags.String("db", "", "path to the SQLite database")
	query := flags.String("query", "", "keyword query")
	limit := flags.Int("limit", 20, "maximum number of results")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *databasePath == "" || *query == "" {
		return errors.New("search requires --db and --query")
	}

	store, err := storage.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	results, err := store.Search(context.Background(), *query, *limit)
	if err != nil {
		return err
	}

	return writeJSON(output, results)
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func defaultValue(environmentName string, fallback string) string {
	if value := os.Getenv(environmentName); value != "" {
		return value
	}
	return fallback
}
