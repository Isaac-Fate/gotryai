package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// FetchPageText downloads plain or source text from a small allowlist of HTTPS hosts
// (GitHub raw / blob URLs). Intended for blogcomposer-style agents that need verifiable code.
type FetchPageText struct {
	Client   *http.Client
	MaxBytes int64
}

type FetchPageOption func(*FetchPageText)

func WithFetchHTTPClient(c *http.Client) FetchPageOption {
	return func(f *FetchPageText) {
		if c != nil {
			f.Client = c
		}
	}
}

func WithFetchMaxBytes(n int64) FetchPageOption {
	return func(f *FetchPageText) {
		if n > 0 {
			f.MaxBytes = n
		}
	}
}

func NewFetchPageText(opts ...FetchPageOption) (*FetchPageText, error) {
	f := &FetchPageText{
		Client: &http.Client{Timeout: 45 * time.Second},
		// Enough for multi-file reads; blogcomposer verifier only needs substantive chunks.
		MaxBytes: 512000,
	}
	for _, o := range opts {
		o(f)
	}
	return f, nil
}

func (f *FetchPageText) Name() string {
	return "Fetch_Page_Text"
}

func (f *FetchPageText) Description() string {
	return "Download source or documentation text from GitHub via HTTPS. " +
		"Pass a full URL: https://github.com/org/repo/blob/branch/path/to/file.go is rewritten to raw content. " +
		"Also accepts https://raw.githubusercontent.com/org/repo/branch/path. " +
		"Use AFTER you have a concrete file URL from search; input must be a single URL string."
}

func (f *FetchPageText) Call(ctx context.Context, input string) (string, error) {
	u := strings.TrimSpace(input)
	if u == "" {
		return "", fmt.Errorf("empty url")
	}
	fetchURL, err := normalizeGitHubFetchURL(u)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ddgChromeUserAgent)
	req.Header.Set("Accept", "text/plain,text/html,*/*;q=0.8")

	resp, err := f.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http %d for %s", resp.StatusCode, fetchURL)
	}
	lim := io.LimitReader(resp.Body, f.MaxBytes+1)
	body, err := io.ReadAll(lim)
	if err != nil {
		return "", err
	}
	if int64(len(body)) > f.MaxBytes {
		return "", fmt.Errorf("response exceeds max bytes (%d)", f.MaxBytes)
	}
	if !utf8.Valid(body) {
		return "", fmt.Errorf("binary or invalid utf-8 response")
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty body)", nil
	}
	return s, nil
}

func normalizeGitHubFetchURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("only https URLs supported")
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "raw.githubusercontent.com", "gist.githubusercontent.com":
		u.Fragment = ""
		return u.String(), nil
	case "github.com":
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		// org/repo/blob/ref/path -> raw.githubusercontent.com/org/repo/ref/path
		if len(parts) >= 4 && parts[2] == "blob" {
			org, repo, ref := parts[0], parts[1], parts[3]
			rest := strings.Join(parts[4:], "/")
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", org, repo, ref, rest), nil
		}
		return "", fmt.Errorf("github URL must be a /blob/ file path, got %q", u.Path)
	default:
		return "", fmt.Errorf("host not allowed (use github.com/blob/... or raw.githubusercontent.com): %s", host)
	}
}
