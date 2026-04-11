package helius

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

const (
	defaultRateRPS   = 500
	defaultRateBurst = 1000
)

type Client struct {
	endpoint string
	fast     *fasthttp.Client
	sem      *semaphore.Weighted
	limit    *rate.Limiter
	mu       sync.Mutex
	nextID   int64
}

func NewFromEnv() (*Client, int, error) {
	endpoint := strings.TrimSpace(os.Getenv("RPC_URL"))
	if endpoint == "" {
		return nil, 0, fmt.Errorf("set RPC_URL to your Solana HTTP JSON-RPC endpoint (e.g. https://mainnet.helius-rpc.com/?api-key=...)")
	}

	rps := float64(defaultRateRPS)
	if s := strings.TrimSpace(os.Getenv("HELIUS_RATE_RPS")); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			rps = v
		}
	}
	burst := defaultRateBurst
	if s := strings.TrimSpace(os.Getenv("HELIUS_RATE_BURST")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			burst = v
		}
	}

	concurrency := int(math.Max(1, math.Round(rps)))
	semWeight := int64(concurrency)

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	fastCl := &fasthttp.Client{
		TLSConfig:                     tlsCfg,
		ReadTimeout:                   120 * time.Second,
		WriteTimeout:                  120 * time.Second,
		MaxIdleConnDuration:           90 * time.Second,
		MaxConnsPerHost:               2048,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
	}

	return &Client{
		endpoint: endpoint,
		fast:     fastCl,
		sem:      semaphore.NewWeighted(semWeight),
		limit:    rate.NewLimiter(rate.Limit(rps), burst),
	}, concurrency, nil
}

func (c *Client) nextJSONRPCID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

func (c *Client) Call(ctx context.Context, method string, params []any, out any) error {
	if err := c.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer c.sem.Release(1)

	if err := c.limit.Wait(ctx); err != nil {
		return err
	}

	id := c.nextJSONRPCID()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	const maxAttempts = 4
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(100*attempt) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		b, status, err := c.postFastHTTP(ctx, raw)
		if err != nil {
			lastErr = err
			continue
		}
		switch status {
		case http.StatusOK:
			return decodeJSONRPCEnvelope(b, out)
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			lastErr = fmt.Errorf("rpc http %d: %s", status, truncate(b, 256))
			continue
		default:
			return fmt.Errorf("rpc http %d: %s", status, truncate(b, 512))
		}
	}
	if lastErr != nil {
		return fmt.Errorf("rpc retries: %w", lastErr)
	}
	return fmt.Errorf("rpc: exhausted retries")
}

func (c *Client) postFastHTTP(ctx context.Context, raw []byte) ([]byte, int, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(c.endpoint)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(raw)

	timeout := 120 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 && d < timeout {
			timeout = d
		}
	}
	err := c.fast.DoTimeout(req, resp, timeout)
	if err != nil {
		return nil, 0, err
	}
	status := resp.StatusCode()
	body := append([]byte(nil), resp.Body()...)
	return body, status, nil
}

func decodeJSONRPCEnvelope(b []byte, out any) error {
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("rpc error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *Client) GetTransactionsForAddressPage(ctx context.Context, address string, opts GetTransactionsForAddressOpts) (*GetTransactionsForAddressResult, error) {
	params := []any{address, opts.ToMap()}
	var res GetTransactionsForAddressResult
	if err := c.Call(ctx, "getTransactionsForAddress", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
