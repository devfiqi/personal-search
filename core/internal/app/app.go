package app

import (
	"fmt"
	"io"
)

const Name = "personal-search-core"

func Run(output io.Writer) error {
	_, err := fmt.Fprintln(output, Name)
	return err
}
