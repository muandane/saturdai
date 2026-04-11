package kubelet

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
)

// Client fetches kubelet stats/summary through the API server node proxy.
type Client struct {
	kubernetes kubernetes.Interface
	timeout    time.Duration
}

// NewClient returns a Client with the given timeout per request.
func NewClient(k kubernetes.Interface, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	return &Client{kubernetes: k, timeout: timeout}
}

// FetchSummary returns stats/summary for a node (GET /api/v1/nodes/{name}/proxy/stats/summary).
func (c *Client) FetchSummary(ctx context.Context, nodeName string) (*Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req := c.kubernetes.CoreV1().RESTClient().Get().
		AbsPath("api/v1/nodes", nodeName, "proxy/stats/summary")
	raw, err := req.DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch summary for node %q: %w", nodeName, err)
	}
	return ParseSummary(raw)
}

// Interface matches Client for testing.
type Interface interface {
	FetchSummary(ctx context.Context, nodeName string) (*Summary, error)
}

var _ Interface = (*Client)(nil)
