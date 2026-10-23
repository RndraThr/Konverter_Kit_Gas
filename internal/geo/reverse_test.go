package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const testAgent = "Konkit/1.0 (test)"

func TestReverseComposesIndonesianAddress(t *testing.T) {
	var query map[string]string
	var agent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query = map[string]string{"format": q.Get("format"), "lang": q.Get("accept-language"), "lat": q.Get("lat"), "lon": q.Get("lon")}
		agent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"address":{"road":"Jl. Merdeka","village":"Patila","county":"Pammana","city":"Kab. Wajo","state":"Sulawesi Selatan","postcode":"90971"}}`))
	}))
	defer server.Close()

	got := NewReverser(server.URL, server.Client(), testAgent).Reverse(context.Background(), -4.0123, 120.1234)
	if want := "Jl. Merdeka, Patila, Pammana, Kab. Wajo, Sulawesi Selatan, 90971"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if query["format"] != "jsonv2" || query["lang"] != "id" || query["lat"] == "" || query["lon"] == "" {
		t.Fatalf("query=%v", query)
	}
	if agent != testAgent {
		t.Fatalf("agent=%q", agent)
	}
}

func TestReverseDeduplicatesParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"address":{"road":"Jl. A","village":"Patila","county":"patila","city":"Wajo","state":"Wajo"}}`))
	}))
	defer server.Close()
	got := NewReverser(server.URL, server.Client(), testAgent).Reverse(context.Background(), 1, 2)
	if want := "Jl. A, Patila, Wajo"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReverseCachesByRoundedCoordinate(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"address":{"road":"Jl. A"}}`))
	}))
	defer server.Close()
	r := NewReverser(server.URL, server.Client(), testAgent)
	a := r.Reverse(context.Background(), -4.01231, 120.12341)
	b := r.Reverse(context.Background(), -4.01234, 120.12344)
	if a != "Jl. A" || b != a || hits != 1 {
		t.Fatalf("a=%q b=%q hits=%d", a, b, hits)
	}
}

func TestReverseReturnsEmptyOnUpstreamError(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		got := NewReverser(server.URL, server.Client(), testAgent).Reverse(context.Background(), 1, 2)
		server.Close()
		if got != "" {
			t.Fatalf("status %d: got %q", status, got)
		}
	}
}

func TestReverseRespectsRateLimit(t *testing.T) {
	var mu sync.Mutex
	var arrivals []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"address":{"road":"Jl. A"}}`))
	}))
	defer server.Close()
	r := NewReverser(server.URL, server.Client(), testAgent)
	r.Reverse(context.Background(), 1, 2)
	r.Reverse(context.Background(), 3, 4)
	if len(arrivals) != 2 {
		t.Fatalf("arrivals=%d", len(arrivals))
	}
	if gap := arrivals[1].Sub(arrivals[0]); gap < 990*time.Millisecond {
		t.Fatalf("gap=%s", gap)
	}
}
