package rss

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const maxImageBytes = 4 << 20

var imageHTTPClient = newImageHTTPClient()

func newImageHTTPClient() *http.Client {
	transport := &http.Transport{
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, a := range ips {
				ip := a.IP
				if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
					return nil, errors.New("图片地址必须为公网地址")
				}
			}
			var last error
			for _, a := range ips {
				conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			if last == nil {
				last = errors.New("图片域名没有可用地址")
			}
			return nil, last
		},
	}
	return &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("图片重定向过多")
		}
		return ValidateFeedURL(r.URL.String())
	}}
}

// Images never use the iframe's network permissions. Each connection is pinned
// to a resolved public IP, including every redirect, to prevent DNS rebinding.
func FetchImage(ctx context.Context, value string) (string, error) {
	return fetchImageWithClient(ctx, value, imageHTTPClient)
}

func fetchImageWithClient(ctx context.Context, value string, client *http.Client) (string, error) {
	if err := ValidateFeedURL(value); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", value, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "DBX RSS Plugin/0.2")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("图片服务器 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return "", err
	}
	return imageDataURL(data)
}
func imageDataURL(data []byte) (string, error) {
	if len(data) > maxImageBytes {
		return "", errors.New("图片超过 4 MiB")
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", errors.New("仅支持 PNG、JPEG、GIF、WebP 图片")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
