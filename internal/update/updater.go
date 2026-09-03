package update

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	GamesURL = "https://github.com/fernandoenzo/nvidia-uwp-patch/raw/main/games.json"
)

// newGamesRequest creates an HTTP GET request for the games.json URL.
func newGamesRequest() (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, GamesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "nvidia-uwp-patch")
	return req, nil
}

// FetchGamesJSON downloads the latest games.json from GitHub.
// Returns the raw JSON bytes or an error if the download fails.
func FetchGamesJSON() ([]byte, error) {
	req, err := newGamesRequest()
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading games.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return body, nil
}
