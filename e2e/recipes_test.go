//go:build e2e

package e2e

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// statusError is a non-2xx response, typed so fetch can tell a 404 from a network blip.
type statusError struct {
	code int
	url  string
}

// Error renders the failing URL and status for the test output.
func (e *statusError) Error() string {
	return fmt.Sprintf("%s: status %d", e.url, e.code)
}

// TestConsumerRecipesStillEmulated fetches each caller's recipe at HEAD and fails when the emulation went stale.
func TestConsumerRecipesStillEmulated(t *testing.T) {
	fetched := map[string]string{}
	for _, c := range []*caller{etkeccPull, etkeccCI, mashRoles, mdadRoles} {
		for _, d := range c.drift {
			content, ok := fetched[d.url]
			if !ok {
				content = fetch(t, d.url)
				fetched[d.url] = content
			}
			for _, want := range d.want {
				if strings.Contains(content, want) {
					continue
				}
				t.Errorf("%s: %s no longer contains %q\nthe emulation is stale: re-read that recipe and update the argv in e2e/consumers_test.go",
					c.repo, d.url, want)
			}
		}
	}
}

// fetch reads one small recipe file, retrying transient failures so a blip is not read as recipe drift.
func fetch(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	for range 3 {
		body, err := get(client, url)
		if err == nil {
			return body
		}
		lastErr = err
		if !transient(err) {
			break
		}
	}
	t.Fatalf("fetching %s failed: %v\nthis is not recipe drift: rerun, and if it keeps failing check the URL", url, lastErr)
	return ""
}

// transient reports whether a fetch error is worth a retry: timeouts and 5xx are, a 404 is not.
func transient(err error) bool {
	var statusErr *statusError
	if errors.As(err, &statusErr) {
		return statusErr.code >= 500
	}
	return true
}

// get performs one request and returns the body of a 200 response.
func get(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &statusError{code: resp.StatusCode, url: url}
	}
	return string(body), nil
}
