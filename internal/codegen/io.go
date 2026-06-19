package codegen

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureOutputDir(file string) error {
	if file == "" {
		return nil
	}

	dir := filepath.Dir(file)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
	}

	return nil
}
