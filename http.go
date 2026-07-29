package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpGet performs a GET with a browser-like User-Agent and a hard timeout.
// A shared client keeps connection reuse across the several source calls.
var httpClient = &http.Client{Timeout: 30 * time.Second}

func httpGet(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Some boards (RemoteOK) reject requests without a real UA.
	req.Header.Set("User-Agent", "JobScout/1.0 (personal daily job digest)")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 40<<20)) // 40 MB cap (ATS boards can be large)
}
