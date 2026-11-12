package rss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxArticlePageBytes = 4 << 20

func FetchArticleContent(ctx context.Context, rawURL string) (string, error) {
	if err := ValidateFeedURL(rawURL); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	transport := &http.Transport{TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 12 * time.Second, DialContext: publicDialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("原文页面重定向次数过多")
		}
		return ValidateFeedURL(req.URL.String())
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DBX RSS Reader/0.4)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取原文页面失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("原文页面返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArticlePageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxArticlePageBytes {
		return "", errors.New("原文页面超过 4 MiB")
	}
	return ExtractArticleContent(bytes.NewReader(data), resp.Request.URL.String())
}

func publicDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, resolved := range ips {
		if !isPublicIP(resolved.IP) {
			return nil, errors.New("原文域名解析到了非公网地址")
		}
	}
	var last error
	for _, resolved := range ips {
		conn, err := (&net.Dialer{Timeout: 6 * time.Second}).DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("原文域名没有可用公网地址")
	}
	return nil, last
}

func ExtractArticleContent(reader io.Reader, pageURL string) (string, error) {
	doc, err := html.Parse(io.LimitReader(reader, maxArticlePageBytes+1))
	if err != nil {
		return "", fmt.Errorf("解析原文页面失败：%w", err)
	}
	var best *html.Node
	bestScore := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "article" || node.Data == "main") {
			score := textLength(node)
			if score > bestScore {
				best, bestScore = node, score
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if best == nil || bestScore < 20 {
		return "", errors.New("原文页面中未找到可提取的正文内容")
	}
	base, _ := url.Parse(pageURL)
	cleanNode(best, base)
	var out bytes.Buffer
	if err := html.Render(&out, best); err != nil {
		return "", err
	}
	return out.String(), nil
}

func textLength(node *html.Node) int {
	if node.Type == html.TextNode {
		return len(strings.TrimSpace(node.Data))
	}
	if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style" || node.Data == "nav" || node.Data == "footer" || node.Data == "header" || node.Data == "aside") {
		return 0
	}
	total := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		total += textLength(child)
	}
	return total
}

func cleanNode(node *html.Node, base *url.URL) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode && (child.Data == "script" || child.Data == "style" || child.Data == "nav" || child.Data == "footer" || child.Data == "header" || child.Data == "aside" || child.Data == "form") {
			node.RemoveChild(child)
		} else {
			cleanNode(child, base)
			if child.Type == html.ElementNode {
				for i := 0; i < len(child.Attr); i++ {
					attr := &child.Attr[i]
					if attr.Key == "src" || attr.Key == "href" {
						if ref, err := url.Parse(strings.TrimSpace(attr.Val)); err == nil && base != nil {
							attr.Val = base.ResolveReference(ref).String()
						}
					} else if attr.Key != "alt" && attr.Key != "title" {
						child.Attr = append(child.Attr[:i], child.Attr[i+1:]...)
						i--
					}
				}
			}
		}
		child = next
	}
}
