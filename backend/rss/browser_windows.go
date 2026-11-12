package rss

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func ListBrowsers() []Browser {
	result := []Browser{{ID: "default", Name: "系统默认浏览器"}}
	candidates := []struct{ id, name, relative string }{
		{"chrome", "Google Chrome", `Google\Chrome\Application\chrome.exe`},
		{"edge", "Microsoft Edge", `Microsoft\Edge\Application\msedge.exe`},
		{"firefox", "Mozilla Firefox", `Mozilla Firefox\firefox.exe`},
	}
	for _, c := range candidates {
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			path := filepath.Join(base, c.relative)
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() {
				result = append(result, Browser{ID: c.id, Name: c.name, Path: path})
				break
			}
		}
	}
	return result
}
func launchBrowser(path, value string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	target := value
	params := ""
	if path != "" {
		target = path
		params = `"` + value + `"`
	}
	// Quotes in a URL must be escaped before passing ShellExecute parameters.
	if path != "" {
		u := stringsReplaceQuotes(value)
		params = `"` + u + `"`
	}
	file, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	args, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return err
	}
	shell := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
	r, _, _ := shell.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 1)
	if r <= 32 {
		return fmt.Errorf("打开浏览器失败（系统错误 %d）", r)
	}
	return nil
}
func stringsReplaceQuotes(value string) string {
	var out []rune
	for _, r := range value {
		if r == '"' {
			out = append(out, '%', '2', '2')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
