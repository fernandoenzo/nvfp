package update

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var GamesURL = "https://github.com/fernandoenzo/nvfp/raw/master/games.json"

// FetchGamesJSON downloads games.json from GitHub and returns its raw JSON bytes.
func FetchGamesJSON() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, GamesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "nvidia-uwp-patch")

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
