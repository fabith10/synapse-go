package agenttools

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fabith10/synapse-go/adk"
)

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

type rssFeedXML struct {
	Channel rssChannel `xml:"channel"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	Link    atomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

type atomFeedXML struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

// GetFetchRSSFeedTool returns a tool to fetch and parse RSS/Atom XML feeds into structured JSON.
func GetFetchRSSFeedTool() adk.Tool {
	return adk.Tool{
		Name:        "fetch_rss_feed",
		Description: "Fetches and parses RSS/Atom XML feeds from blogs, GitHub releases, news channels, or advisories into structured JSON.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Full HTTP/HTTPS URL of the RSS or Atom XML feed",
				},
				"max_items": map[string]interface{}{
					"type":        "integer",
					"description": "Max feed items to return (default 10, max 50)",
				},
			},
			"required": []interface{}{"url"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				URL      string `json:"url"`
				MaxItems int    `json:"max_items"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.URL == "" {
				return "", fmt.Errorf("url parameter is required")
			}
			if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
				params.URL = "https://" + params.URL
			}

			maxItems := params.MaxItems
			if maxItems <= 0 {
				maxItems = 10
			} else if maxItems > 50 {
				maxItems = 50
			}

			execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(execCtx, "GET", params.URL, nil)
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("User-Agent", "SynapseGo-FeedFetcher/1.0")

			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("failed to fetch RSS feed from %s: %w", params.URL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("feed server returned HTTP %d", resp.StatusCode)
			}

			bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
			if err != nil {
				return "", fmt.Errorf("failed to read feed response: %w", err)
			}

			// Try RSS 2.0 parsing first
			var rssData rssFeedXML
			if err := xml.Unmarshal(bodyBytes, &rssData); err == nil && len(rssData.Channel.Items) > 0 {
				items := rssData.Channel.Items
				if len(items) > maxItems {
					items = items[:maxItems]
				}

				var parsedItems []map[string]string
				for _, item := range items {
					parsedItems = append(parsedItems, map[string]string{
						"title":       strings.TrimSpace(item.Title),
						"link":        strings.TrimSpace(item.Link),
						"pub_date":    strings.TrimSpace(item.PubDate),
						"description": strings.TrimSpace(item.Description),
					})
				}

				outBytes, _ := json.MarshalIndent(map[string]interface{}{
					"feed_type":   "RSS 2.0",
					"feed_title":  rssData.Channel.Title,
					"feed_link":   rssData.Channel.Link,
					"item_count":  len(parsedItems),
					"items":       parsedItems,
				}, "", "  ")
				return string(outBytes), nil
			}

			// Try Atom XML parsing fallback
			var atomData atomFeedXML
			if err := xml.Unmarshal(bodyBytes, &atomData); err == nil && len(atomData.Entries) > 0 {
				entries := atomData.Entries
				if len(entries) > maxItems {
					entries = entries[:maxItems]
				}

				var parsedItems []map[string]string
				for _, entry := range entries {
					parsedItems = append(parsedItems, map[string]string{
						"title":       strings.TrimSpace(entry.Title),
						"link":        strings.TrimSpace(entry.Link.Href),
						"pub_date":    strings.TrimSpace(entry.Updated),
						"description": strings.TrimSpace(entry.Summary),
					})
				}

				outBytes, _ := json.MarshalIndent(map[string]interface{}{
					"feed_type":  "Atom",
					"feed_title": atomData.Title,
					"item_count": len(parsedItems),
					"items":      parsedItems,
				}, "", "  ")
				return string(outBytes), nil
			}

			return "", fmt.Errorf("unable to parse XML as RSS or Atom feed")
		},
	}
}
