package testutil

import (
	"os"
	"path/filepath"
	"puffin/pkg/assert"
	"runtime"
)

func ReadTestdata(name string) ([]byte, error) {
	assert.Assert(name != "", "name is empty")
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, os.ErrNotExist
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return os.ReadFile(filepath.Join(root, "testdata", name))
}
