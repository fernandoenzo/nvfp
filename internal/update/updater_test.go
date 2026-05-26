package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchGamesJSON_Success(t *testing.T) {
	expected := []byte(`{"version":1,"games":[{"fingerprint":"test","app_id":"Pkg!App"}]}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(expected)
	}))
	defer server.Close()

	// Temporarily replace GamesURL
	originalURL := GamesURL
	GamesURL = server.URL
	defer func() { GamesURL = originalURL }()

	data, err := FetchGamesJSON()
	if err != nil {
		t.Fatalf("FetchGamesJSON() error: %v", err)
	}
	if string(data) != string(expected) {
		t.Errorf("FetchGamesJSON() = %q, want %q", string(data), string(expected))
	}
}

func TestFetchGamesJSON_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalURL := GamesURL
	GamesURL = server.URL
	defer func() { GamesURL = originalURL }()

	_, err := FetchGamesJSON()
	if err == nil {
		t.Error("FetchGamesJSON() expected error for HTTP 500, got nil")
	}
}

func TestFetchGamesJSON_NetworkError(t *testing.T) {
	// Use a server that immediately closes connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler is never reached because we use a closed server
	}))
	server.Close()

	originalURL := GamesURL
	GamesURL = server.URL
	defer func() { GamesURL = originalURL }()

	_, err := FetchGamesJSON()
	if err == nil {
		t.Error("FetchGamesJSON() expected error for network failure, got nil")
	}
}