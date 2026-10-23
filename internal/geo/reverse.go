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

	limitMu sync.Mutex
	next    time.Time // earliest start of the next upstream call

	flightMu sync.Mutex
	inflight map[string]*flight
}

// flight is one in-progress upstream lookup shared by concurrent callers.
type flight struct {
	done   chan struct{}
	result string
}

// NewReverser builds a Reverser. A nil client gets a 5 second timeout.
func NewReverser(baseURL string, client *http.Client, userAgent string) *Reverser {
	if client == nil {
		client = &http.Client{Timeout: clientTimeout}
	}
	return &Reverser{baseURL: strings.TrimRight(baseURL, "/"), client: client, userAgent: userAgent, cache: map[string]string{}, inflight: map[string]*flight{}}
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
	r.flightMu.Lock()
	if call, ok := r.inflight[key]; ok {
		r.flightMu.Unlock()
		select {
		case <-call.done:
			return call.result
		case <-ctx.Done():
			return ""
		}
	}
	call := &flight{done: make(chan struct{})}
	r.inflight[key] = call
	r.flightMu.Unlock()

	call.result = r.fetch(ctx, lat, lng)
	if call.result != "" {
		r.cacheMu.Lock()
		if len(r.cache) >= cacheLimit {
			r.cache = map[string]string{}
		}
		r.cache[key] = call.result
		r.cacheMu.Unlock()
	}
	r.flightMu.Lock()
	delete(r.inflight, key)
	r.flightMu.Unlock()
	close(call.done)
	return call.result
}

// wait reserves the next upstream slot (>= 1 s after the previous one) under a
// short lock, then sleeps outside it so a cancelled caller leaves promptly.
func (r *Reverser) wait(ctx context.Context) bool {
	r.limitMu.Lock()
	now := time.Now()
	slot := r.next
	if slot.Before(now) {
		slot = now
	}
	r.next = slot.Add(minCallGap)
	r.limitMu.Unlock()
	delay := slot.Sub(now)
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
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
