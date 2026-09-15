package dexgithub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const maxAPIResponseBytes = 8 << 20

// APIError is a sanitized GitHub API failure. It never contains request
// authorization headers or tokens.
type APIError struct {
	StatusCode    int
	Message       string
	RequestID     string
	RetryAfter    time.Duration
	RateRemaining int
	RateReset     time.Time
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub API respondió HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("GitHub API respondió HTTP %d: %s", e.StatusCode, e.Message)
}

type apiClient struct {
	cfg Config
}

func newAPIClient(cfg Config) *apiClient { return &apiClient{cfg: cfg} }

func (c *apiClient) apiURL(endpoint string) (string, error) {
	return joinBaseURL(c.cfg.APIBaseURL, endpoint)
}

func (c *apiClient) webURL(endpoint string) (string, error) {
	return joinBaseURL(c.cfg.WebBaseURL, endpoint)
}

func joinBaseURL(base, endpoint string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimSuffix(u.Path, "/")
	u.Path = path.Clean(basePath + "/" + strings.TrimPrefix(endpoint, "/"))
	if u.Path == "." {
		u.Path = "/"
	}
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (c *apiClient) requestJSON(ctx context.Context, method, rawURL, bearer string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", c.cfg.APIVersion)
	req.Header.Set("User-Agent", "dexgithub")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxAPIResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxAPIResponseBytes {
		return errors.New("dexgithub: respuesta de GitHub excede 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp, data)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decodificar respuesta GitHub: %w", err)
	}
	return nil
}

func parseAPIError(resp *http.Response, data []byte) error {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &payload)
	if len(payload.Message) > 512 {
		payload.Message = payload.Message[:512]
	}
	err := &APIError{
		StatusCode:    resp.StatusCode,
		Message:       strings.TrimSpace(payload.Message),
		RequestID:     resp.Header.Get("X-GitHub-Request-Id"),
		RateRemaining: -1,
	}
	if seconds, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && seconds >= 0 {
		err.RetryAfter = time.Duration(seconds) * time.Second
	}
	if remaining, convErr := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining")); convErr == nil {
		err.RateRemaining = remaining
	}
	if reset, convErr := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); convErr == nil && reset > 0 {
		err.RateReset = time.Unix(reset, 0)
	}
	return err
}
