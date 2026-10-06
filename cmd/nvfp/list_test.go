package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestListGames(t *testing.T) {
	gameDB := newTestGameDB()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listGames(gameDB)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "final_fantasy_vii_remake") {
		t.Error("listGames() output missing fingerprint name")
	}
	if !strings.Contains(output, "Games database version: 1") {
		t.Error("listGames() output missing version line")
	}
	if !strings.Contains(output, "Total games: 3") {
		t.Error("listGames() output missing total games count")
	}
}
