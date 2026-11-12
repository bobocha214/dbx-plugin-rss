package rss

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

const maxFeedBytes = 5 << 20

type Feed struct {
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Link        string    `json:"link,omitempty"`
	Items       []Article `json:"items"`
}

type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Link        string    `json:"link,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Content     string    `json:"content,omitempty"`
	Author      string    `json:"author,omitempty"`
	PublishedAt time.Time `json:"publishedAt,omitempty"`
	ImageURL    string    `json:"imageUrl,omitempty"`
}

type mediaItem struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}
type mediaFields struct {
	Enclosures []mediaItem `xml:"enclosure"`
	Media      []mediaItem `xml:"http://search.yahoo.com/mrss/ content"`
	Thumbnails []mediaItem `xml:"http://search.yahoo.com/mrss/ thumbnail"`
}

func (m mediaFields) image(base string) string {
	for _, items := range [][]mediaItem{m.Media, m.Enclosures, m.Thumbnails} {
		for _, item := range items {
			if item.URL != "" && (strings.HasPrefix(item.Type, "image/") || item.Medium == "image" || item.Type == "" && item.Medium == "") {
				return resolveLink(base, item.URL)
			}
		}
	}
	return ""
}

type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
	HTML string `xml:",innerxml"`
}

func (t atomText) value() string {
	if t.Type == "xhtml" {
		return t.HTML
	}
	return t.Text
}

type rssDocument struct {
	Channel struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Link        string `xml:"link"`
		Items       []struct {
			mediaFields
			GUID        string `xml:"guid"`
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
			Author      string `xml:"author"`
			Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type rdfDocument struct {
	Channel struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Link        string `xml:"link"`
	} `xml:"channel"`
	Items []struct {
		mediaFields
		ID          string `xml:"about,attr"`
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
		Date        string `xml:"http://purl.org/dc/elements/1.1/ date"`
	} `xml:"item"`
}

type atomDocument struct {
	Title    string     `xml:"title"`
	Subtitle string     `xml:"subtitle"`
	Links    []atomLink `xml:"link"`
	Entries  []struct {
		ID      string   `xml:"id"`
		Title   string   `xml:"title"`
		Updated string   `xml:"updated"`
		Created string   `xml:"published"`
		Summary atomText `xml:"summary"`
		Content atomText `xml:"content"`
		mediaFields
		Author struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Links []atomLink `xml:"link"`
	} `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

func Parse(reader io.Reader, sourceURL string) (Feed, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxFeedBytes+1))
	if err != nil {
		return Feed{}, fmt.Errorf("read feed: %w", err)
	}
	if len(data) > maxFeedBytes {
		return Feed{}, errors.New("feed exceeds the 5 MiB size limit")
	}
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return Feed{}, fmt.Errorf("parse XML: %w", err)
	}
	switch strings.ToLower(root.XMLName.Local) {
	case "rss":
		var doc rssDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return Feed{}, fmt.Errorf("parse RSS: %w", err)
		}
		return fromRSS(doc, sourceURL), nil
	case "rdf":
		var doc rdfDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return Feed{}, fmt.Errorf("parse RDF: %w", err)
		}
		return fromRDF(doc, sourceURL), nil
	case "feed":
		var doc atomDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return Feed{}, fmt.Errorf("parse Atom: %w", err)
		}
		return fromAtom(doc, sourceURL), nil
	default:
		return Feed{}, fmt.Errorf("unsupported feed document %q", root.XMLName.Local)
	}
}

func fromRDF(doc rdfDocument, sourceURL string) Feed {
	feed := Feed{Title: cleanText(doc.Channel.Title), Description: cleanText(doc.Channel.Description), Link: resolveLink(sourceURL, doc.Channel.Link)}
	feed.Items = make([]Article, 0, len(doc.Items))
	for _, raw := range doc.Items {
		link := resolveLink(sourceURL, raw.Link)
		published := parseDate(raw.Date)
		feed.Items = append(feed.Items, Article{ID: stableID(firstNonEmpty(raw.ID, link), raw.Title, published), Title: cleanText(raw.Title), Link: link, Summary: raw.Description, Content: raw.Description, Author: cleanText(raw.Creator), PublishedAt: published, ImageURL: raw.mediaFields.image(sourceURL)})
	}
	return feed
}

func fromRSS(doc rssDocument, sourceURL string) Feed {
	feed := Feed{Title: cleanText(doc.Channel.Title), Description: cleanText(doc.Channel.Description), Link: resolveLink(sourceURL, doc.Channel.Link)}
	feed.Items = make([]Article, 0, len(doc.Channel.Items))
	for _, raw := range doc.Channel.Items {
		link := resolveLink(sourceURL, raw.Link)
		content := raw.Content
		if content == "" {
			content = raw.Description
		}
		published := parseDate(raw.PubDate)
		id := strings.TrimSpace(raw.GUID)
		if id == "" {
			id = link
		}
		feed.Items = append(feed.Items, Article{
			ID: stableID(id, cleanText(raw.Title), published), Title: cleanText(raw.Title), Link: link,
			Summary: raw.Description, Content: content, Author: cleanText(firstNonEmpty(raw.Creator, raw.Author)), PublishedAt: published, ImageURL: raw.mediaFields.image(sourceURL),
		})
	}
	return feed
}

func fromAtom(doc atomDocument, sourceURL string) Feed {
	feed := Feed{Title: cleanText(doc.Title), Description: cleanText(doc.Subtitle), Link: resolveLink(sourceURL, atomHref(doc.Links))}
	feed.Items = make([]Article, 0, len(doc.Entries))
	for _, raw := range doc.Entries {
		published := parseDate(firstNonEmpty(raw.Created, raw.Updated))
		link := resolveLink(sourceURL, atomHref(raw.Links))
		image := raw.mediaFields.image(sourceURL)
		for _, l := range raw.Links {
			if image == "" && l.Rel == "enclosure" && strings.HasPrefix(l.Type, "image/") {
				image = resolveLink(sourceURL, l.Href)
			}
		}
		feed.Items = append(feed.Items, Article{
			ID: stableID(raw.ID, link, published), Title: cleanText(raw.Title), Link: link,
			Summary: raw.Summary.value(), Content: raw.Content.value(), Author: cleanText(raw.Author.Name), PublishedAt: published, ImageURL: image,
		})
	}
	return feed
}

func ValidateFeedURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return errors.New("请输入有效的 RSS/Atom 地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("订阅地址必须是 HTTP 或 HTTPS URL，且不能包含账号密码")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errors.New("不允许访问本机或局域网订阅地址")
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return errors.New("不允许访问本机或局域网订阅地址")
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func stableID(id, link string, published time.Time) string {
	value := strings.TrimSpace(id)
	if value == "" {
		value = strings.TrimSpace(link)
	}
	if value == "" {
		value = published.UTC().Format(time.RFC3339Nano)
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func resolveLink(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return value
	}
	ref, err := url.Parse(value)
	if err != nil {
		return value
	}
	return baseURL.ResolveReference(ref).String()
}

func atomHref(links []atomLink) string {
	for _, link := range links {
		if link.Rel == "" || link.Rel == "alternate" {
			return link.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, time.ANSIC} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func cleanText(value string) string { return strings.TrimSpace(value) }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
