package gifs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrNotConfigured means no API key was supplied.
var ErrNotConfigured = errors.New("gif search is not configured")

// ErrUpstream means the provider was unreachable or returned an error.
var ErrUpstream = errors.New("gif provider unavailable")

type Result struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

type Response struct {
	Results []Result `json:"results"`
	Next    *string  `json:"next"`
}

// Client talks to Klipy's Tenor-compatible API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://api.klipy.com/v2",
		http: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) Configured() bool { return c != nil && c.apiKey != "" }

func (c *Client) Search(ctx context.Context, query string, limit int, pos string) (*Response, error) {
	params := url.Values{"q": {query}}
	return c.fetch(ctx, "/search", params, limit, pos)
}

func (c *Client) Trending(ctx context.Context, limit int, pos string) (*Response, error) {
	return c.fetch(ctx, "/featured", url.Values{}, limit, pos)
}

func (c *Client) fetch(ctx context.Context, path string, params url.Values, limit int, pos string) (*Response, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	params.Set("key", c.apiKey)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("media_filter", "gif,tinygif")
	params.Set("contentfilter", "medium")
	if pos != "" {
		params.Set("pos", pos)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var payload struct {
		Next    string `json:"next"`
		Results []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			MediaFormats map[string]struct {
				URL  string `json:"url"`
				Dims []int  `json:"dims"`
			} `json:"media_formats"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	out := &Response{Results: []Result{}}
	if payload.Next != "" {
		next := payload.Next
		out.Next = &next
	}

	for _, item := range payload.Results {
		gif, ok := item.MediaFormats["gif"]
		if !ok || gif.URL == "" {
			continue
		}
		preview := gif.URL
		if tiny, ok := item.MediaFormats["tinygif"]; ok && tiny.URL != "" {
			preview = tiny.URL
		}
		w, h := 0, 0
		if len(gif.Dims) >= 2 {
			w, h = gif.Dims[0], gif.Dims[1]
		}
		out.Results = append(out.Results, Result{
			ID:         item.ID,
			Title:      item.Title,
			URL:        gif.URL,
			PreviewURL: preview,
			Width:      w,
			Height:     h,
		})
	}
	return out, nil
}
