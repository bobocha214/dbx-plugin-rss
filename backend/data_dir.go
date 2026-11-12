package main

import (
	"errors"
	"os"
)

// Read the host contract directly so packaging with older bundled SDKs works.
func ensurePluginDataDir() (string, error) {
	dir := os.Getenv("DBX_PLUGIN_DATA_DIR")
	if dir == "" {
		return "", errors.New("plugin data directory is not set; running outside a DBX host")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
