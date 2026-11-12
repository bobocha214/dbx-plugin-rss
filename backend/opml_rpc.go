package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/example/dbx-plugin-rss/rss"
	dbxpluginsdk "github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk"
	"os"
	"strconv"
	"time"
)

func (p *Plugin) opmlRPC(method string, params json.RawMessage) (any, *dbxpluginsdk.PluginError) {
	if method == "rss/importOPMLStart" {
		var in struct {
			Content string `json:"content"`
			Base64  string `json:"base64"`
		}
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, invalidParams("invalid OPML")
		}
		data := []byte(in.Content)
		if in.Base64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(in.Base64)
			if err != nil {
				return nil, invalidParams("invalid OPML base64")
			}
			data = decoded
		}
		feeds, err := rss.ParseOPML(data)
		if err != nil {
			return nil, rssError(err)
		}
		job := &opmlImportJob{Total: len(feeds)}
		p.jobsMu.Lock()
		if p.importJobs == nil {
			p.importJobs = make(map[string]*opmlImportJob)
		}
		id := strconv.FormatInt(time.Now().UnixNano(), 10)
		p.importJobs[id] = job
		p.jobsMu.Unlock()
		go func() {
			result, err := p.service.ImportOPMLWithProgress(context.Background(), data, func(done, total int, url string) {
				job.mu.Lock()
				job.Current, job.Total, job.URL = done, total, url
				job.mu.Unlock()
			})
			job.mu.Lock()
			job.Done = true
			job.URL = ""
			if err != nil {
				job.Error = err.Error()
			} else {
				job.Result = &result
			}
			job.mu.Unlock()
		}()
		return map[string]any{"jobId": id, "total": len(feeds)}, nil
	}
	if method == "rss/importOPMLStatus" {
		var in struct {
			JobID string `json:"jobId"`
		}
		if err := json.Unmarshal(params, &in); err != nil || in.JobID == "" {
			return nil, invalidParams("jobId is required")
		}
		p.jobsMu.Lock()
		job := p.importJobs[in.JobID]
		p.jobsMu.Unlock()
		if job == nil {
			return nil, invalidParams("导入任务不存在")
		}
		job.mu.RLock()
		defer job.mu.RUnlock()
		out := map[string]any{"done": job.Done, "current": job.Current, "total": job.Total, "url": job.URL}
		if job.Error != "" {
			out["error"] = job.Error
		}
		if job.Result != nil {
			out["result"] = job.Result
		}
		return out, nil
	}
	if method == "rss/exportOPMLFile" {
		var in struct {
			FeedIDs  []string `json:"feedIds"`
			Filename string   `json:"filename"`
		}
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, invalidParams("invalid feedIds")
		}
		content, err := p.service.ExportOPML(in.FeedIDs)
		if err != nil {
			return nil, rssError(err)
		}
		path, ok, err := chooseOPMLExportPath(in.Filename)
		if err != nil {
			return nil, rssError(err)
		}
		if !ok {
			return map[string]any{"cancelled": true}, nil
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return nil, rssError(fmt.Errorf("write OPML: %w", err))
		}
		return map[string]string{"filename": path, "path": path}, nil
	}
	if method == "rss/exportOPML" {
		var in struct {
			FeedIDs []string `json:"feedIds"`
		}
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, invalidParams("invalid feedIds")
		}
		out, err := p.service.ExportOPML(in.FeedIDs)
		if err != nil {
			return nil, rssError(err)
		}
		return map[string]string{"filename": "rss-subscriptions.opml", "content": out}, nil
	}
	var in struct {
		Content string `json:"content"`
		Base64  string `json:"base64"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, invalidParams("invalid OPML")
	}
	data := []byte(in.Content)
	if in.Base64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(in.Base64)
		if err != nil {
			return nil, invalidParams("invalid OPML base64")
		}
		data = decoded
	}
	if method == "rss/previewOPML" {
		feeds, err := rss.ParseOPML(data)
		if err != nil {
			return nil, rssError(err)
		}
		return feeds, nil
	}
	result, err := p.service.ImportOPML(context.Background(), data)
	if err != nil {
		return nil, rssError(err)
	}
	return result, nil
}
