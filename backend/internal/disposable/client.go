// Package disposable talks to the docker-controller sidecar, the only service
// allowed to create and destroy disposable Docker containers.
package disposable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is an authenticated HTTP client for the docker-controller.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Container describes a freshly created disposable container.
type Container struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	CreatedAt int64  `json:"createdAt"`
}

// ErrNotConfigured is returned when the sidecar is not configured for this
// deployment (the feature is disabled, not broken).
var ErrNotConfigured = errors.New("disposable containers are not configured")

// StatusError is a non-2xx answer from the sidecar.
type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string { return e.Msg }

// FromEnv builds a client from CONTROLLER_URL and CONTROLLER_TOKEN.
func FromEnv() *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("CONTROLLER_URL")), "/"),
		token:   strings.TrimSpace(os.Getenv("CONTROLLER_TOKEN")),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Configured reports whether the sidecar's address and token are set.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

// Health verifies that the sidecar is up and can reach Docker. It is what
// decides whether the "disposable container" button is offered.
func (c *Client) Health(ctx context.Context) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var out struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, http.MethodGet, "/health", nil, &out); err != nil {
		return err
	}
	if out.Status != "ok" {
		return fmt.Errorf("docker-controller is unhealthy")
	}
	return nil
}

// Create starts a new disposable container. owner is recorded as a label.
func (c *Client) Create(ctx context.Context, owner string) (*Container, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	body, err := json.Marshal(map[string]string{"owner": owner})
	if err != nil {
		return nil, err
	}
	var out Container
	if err := c.call(ctx, http.MethodPost, "/containers", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a disposable container and its data volume.
func (c *Client) Delete(ctx context.Context, containerID string) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	if containerID == "" {
		return errors.New("container id is empty")
	}
	return c.call(ctx, http.MethodDelete, "/containers/"+containerID, nil, nil)
}

func (c *Client) call(ctx context.Context, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker-controller unreachable: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			return fmt.Errorf("%w: %s", ErrNotConfigured, e.Error)
		}
		if e.Error == "" {
			e.Error = fmt.Sprintf("docker-controller returned %d", resp.StatusCode)
		}
		return &StatusError{Code: resp.StatusCode, Msg: e.Error}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
