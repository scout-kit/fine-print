package api

import (
	"context"
	"os"
)

func contextBackground() context.Context {
	return context.Background()
}

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}
