//go:build !windows

package main

import "errors"

func chooseOPMLExportPath(string) (string, bool, error) {
	return "", false, errors.New("OPML export save dialog is only available on Windows")
}
