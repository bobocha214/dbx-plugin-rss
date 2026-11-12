//go:build !windows

package rss

import (
	"errors"
	"os/exec"
	"runtime"
)

func ListBrowsers() []Browser { return []Browser{{ID: "default", Name: "系统默认浏览器"}} }
func launchBrowser(path, value string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("当前平台暂不支持浏览器打开")
	}
	cmd := exec.Command(command, value)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
