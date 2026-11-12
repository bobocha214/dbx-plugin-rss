package rss

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

type Browser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"-"`
}

func validBrowser(id string) bool {
	return id == "default" || id == "chrome" || id == "edge" || id == "firefox"
}
func ValidateBrowserURL(value string) error {
	if len(value) > 4096 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("原文地址无效")
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("仅支持 HTTP / HTTPS 原文地址")
	}
	return nil
}
func OpenBrowser(value, id string) error {
	if err := ValidateBrowserURL(value); err != nil {
		return err
	}
	if !validBrowser(id) {
		return errors.New("不支持的浏览器")
	}
	for _, b := range ListBrowsers() {
		if b.ID == id {
			return launchBrowser(b.Path, value)
		}
	}
	return errors.New("所选浏览器未安装，请在设置中重新选择")
}
