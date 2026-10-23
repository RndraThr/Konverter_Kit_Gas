// Package geo resolves coordinates to human-readable Indonesian addresses via
// a Nominatim-compatible reverse-geocoding service.
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	cacheLimit     = 5000
	minCallGap     = time.Second
	clientTimeout  = 5 * time.Second
	maxResponseLen = 1 << 20
)

// Reverser is a caching, rate-limited reverse geocoder.
type Reverser struct {
	baseURL   string
	client    *http.Client
	userAgent string

	cacheMu sync.Mutex
	cache   map[string]string

	limitMu  sync.Mutex
	lastCall time.Time
}

// NewReverser builds a Reverser. A nil client gets a 5 second timeout.
func NewReverser(baseURL string, client *http.Client, userAgent string) *Reverser {
	if client == nil {
		client = &http.Client{Timeout: clientTimeout}
	}
	return &Reverser{baseURL: strings.TrimRight(baseURL, "/"), client: client, userAgent: userAgent, cache: map[string]string{}}
}

// Reverse returns the address for the coordinate, or "" on any upstream failure.
func (r *Reverser) Reverse(ctx context.Context, lat, lng float64) string {
	key := fmt.Sprintf("%.4f,%.4f", lat, lng)
	r.cacheMu.Lock()
	cached, ok := r.cache[key]
	r.cacheMu.Unlock()
	if ok {
		return cached
	}
	address := r.fetch(ctx, lat, lng)
	if address == "" {
		return ""
	}
	r.cacheMu.Lock()
	if len(r.cache) >= cacheLimit {
		r.cache = map[string]string{}
	}
	r.cache[key] = address
	r.cacheMu.Unlock()
	return address
}

// wait enforces the minimum gap between upstream calls (Nominatim policy).
func (r *Reverser) wait(ctx context.Context) bool {
	r.limitMu.Lock()
	defer r.limitMu.Unlock()
	if delay := minCallGap - time.Since(r.lastCall); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return false
		}
	}
	r.lastCall = time.Now()
	return true
}

func (r *Reverser) fetch(ctx context.Context, lat, lng float64) string {
	if !r.wait(ctx) {
		return ""
	}
	query := url.Values{}
	query.Set("format", "jsonv2")
	query.Set("accept-language", "id")
	query.Set("lat", fmt.Sprintf("%.6f", lat))
	query.Set("lon", fmt.Sprintf("%.6f", lng))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/reverse?"+query.Encode(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", r.userAgent)
	resp, err := r.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Address map[string]string `json:"address"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseLen)).Decode(&body); err != nil {
		return ""
	}
	return compose(body.Address)
}

var addressFields = [][]string{
	{"road"},
	{"neighbourhood", "suburb", "village"},
	{"city_district", "county", "municipality"},
	{"city", "town", "regency", "county"},
	{"state"},
	{"postcode"},
}

func compose(address map[string]string) string {
	seen := map[string]bool{}
	var parts []string
	for _, group := range addressFields {
		for _, key := range group {
			value := strings.TrimSpace(address[key])
			if value == "" {
				continue
			}
			if folded := strings.ToLower(value); !seen[folded] {
				seen[folded] = true
				parts = append(parts, value)
			}
			break
		}
	}
	return strings.Join(parts, ", ")
}
