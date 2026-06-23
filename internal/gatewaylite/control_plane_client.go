package gatewaylite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ControlPlaneClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

func NewControlPlaneClient(baseURL, authToken string, timeout time.Duration) (*ControlPlaneClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("control plane base URL is required")
	}
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	}
	return &ControlPlaneClient{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *ControlPlaneClient) ResolveKey(ctx context.Context, req ResolveKeyRequest) (*ResolveKeyResponse, error) {
	var out ResolveKeyResponse
	if err := c.postJSON(ctx, "/internal/key/resolve", req, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return &out, nil
	}
	return &out, nil
}

func (c *ControlPlaneClient) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (*AcquireLeaseResponse, error) {
	var out AcquireLeaseResponse
	if err := c.postJSON(ctx, "/internal/quota/acquire-lease", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) RefillLease(ctx context.Context, req AcquireLeaseRequest) (*AcquireLeaseResponse, error) {
	var out AcquireLeaseResponse
	if err := c.postJSON(ctx, "/internal/quota/refill-lease", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) RebalanceLease(ctx context.Context, req RebalanceLeaseRequest) (*RebalanceLeaseResponse, error) {
	var out RebalanceLeaseResponse
	if err := c.postJSON(ctx, "/internal/quota/rebalance-lease", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) ReportUsage(ctx context.Context, event UsageEvent) error {
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := c.postJSON(ctx, "/internal/usage/report", event, &out); err != nil {
		return err
	}
	if !out.OK {
		if out.Error == "" {
			out.Error = "usage report rejected"
		}
		return errors.New(out.Error)
	}
	return nil
}

func (c *ControlPlaneClient) ReportUsageBatch(ctx context.Context, events []UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	var out UsageBatchReportResponse
	if err := c.postJSON(ctx, "/internal/usage/report-batch", UsageBatchReportRequest{Events: events}, &out); err != nil {
		return err
	}
	if !out.OK {
		if out.Error == "" {
			out.Error = "usage batch report rejected"
		}
		return errors.New(out.Error)
	}
	return nil
}

func (c *ControlPlaneClient) ReportGatewayHealth(ctx context.Context, req GatewayHealthReportRequest) (*GatewayHealthReportResponse, error) {
	var out GatewayHealthReportResponse
	if err := c.postJSON(ctx, "/internal/gateway/health-report", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) ProbeControlPlaneHealth(ctx context.Context) (time.Duration, error) {
	startedAt := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("control plane /health returned %d", resp.StatusCode)
	}
	return time.Since(startedAt), nil
}

func (c *ControlPlaneClient) FetchGatewayConfigSnapshot(ctx context.Context, req GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error) {
	var out GatewayConfigSnapshotResponse
	if err := c.postJSON(ctx, "/internal/config/snapshot", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) FetchCacheInvalidations(ctx context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
	var out CacheInvalidationResponse
	if err := c.postJSON(ctx, "/internal/cache/invalidate", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ControlPlaneClient) postJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("control plane %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
