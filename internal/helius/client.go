package helius

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

const defaultRateRPS = 500

type Client struct {
	endpoint string
	http     *http.Client
	sem      *semaphore.Weighted
	limit    *rate.Limiter
	nextID   atomic.Int64
}

func NewFromEnv() (*Client, int, error) {
	endpoint := strings.TrimSpace(os.Getenv("RPC_URL"))
	if endpoint == "" {
		return nil, 0, fmt.Errorf("set RPC_URL to your Solana HTTP JSON-RPC endpoint (e.g. https://mainnet.helius-rpc.com/?api-key=...)")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, 0, fmt.Errorf("RPC_URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, 0, fmt.Errorf("RPC_URL scheme must be http or https")
	}

	rps := float64(defaultRateRPS)
	if s := strings.TrimSpace(os.Getenv("HELIUS_RATE_RPS")); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			rps = v
		}
	}

	lim := rate.NewLimiter(rate.Inf, 0)
	concurrency := max(64, int(math.Round(rps)))
	semWeight := int64(concurrency)

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(128),
		NextProtos:         []string{"h2", "http/1.1"},
		CurvePreferences:   []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	var tr *http.Transport
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = t.Clone()
	} else {
		tr = &http.Transport{}
	}

	tr.DisableCompression = true
	tr.MaxIdleConns = 512
	tr.MaxIdleConnsPerHost = 256
	tr.IdleConnTimeout = 5 * time.Minute
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 60 * time.Second
	tr.ReadBufferSize = 64 * 1024
	tr.WriteBufferSize = 16 * 1024

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		setTCPNoDelay(conn)
		return conn, nil
	}

	if strings.EqualFold(u.Scheme, "https") {
		tr.TLSClientConfig = tlsCfg
		tr.ForceAttemptHTTP2 = true
	} else {
		tr.TLSClientConfig = nil
		tr.ForceAttemptHTTP2 = false
	}

	httpCl := &http.Client{
		Transport: tr,
		Timeout:   0,
	}

	return &Client{
		endpoint: endpoint,
		http:     httpCl,
		sem:      semaphore.NewWeighted(semWeight),
		limit:    lim,
	}, concurrency, nil
}

func setTCPNoDelay(c net.Conn) {
	for c != nil {
		switch t := c.(type) {
		case *net.TCPConn:
			_ = t.SetNoDelay(true)
			return
		case interface{ NetConn() net.Conn }:
			c = t.NetConn()
		default:
			return
		}
	}
}

func (c *Client) Call(ctx context.Context, method string, params []any, out any) error {
	if err := c.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer c.sem.Release(1)

	if err := c.limit.Wait(ctx); err != nil {
		return err
	}

	id := c.nextID.Add(1)
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	raw, err := sonic.Marshal(body)
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

		b, status, err := c.postHTTP(ctx, raw)
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

func (c *Client) postHTTP(ctx context.Context, raw []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(raw))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
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
	if err := sonic.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("rpc error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if out == nil {
		return nil
	}
	if err := sonic.Unmarshal(envelope.Result, out); err != nil {
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

func (c *Client) Warmup(ctx context.Context) error {
	var out string
	return c.Call(ctx, "getHealth", []any{}, &out)
}
