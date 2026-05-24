package update

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	GamesURL = "https://github.com/fernandoenzo/nvidia-uwp-patch/raw/main/games.json"
)

// FetchGamesJSON downloads the latest games.json from GitHub.
// Returns the raw JSON bytes or an error if the download fails.
func FetchGamesJSON() ([]byte, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(GamesURL)
	if err != nil {
		return nil, fmt.Errorf("downloading games.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}