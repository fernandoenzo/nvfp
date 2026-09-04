build:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o nvidia-uwp-patch.exe .

test:
	go test ./...

.PHONY: build test