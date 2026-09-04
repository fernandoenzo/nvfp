build:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o nvfp.exe .

test:
	go test ./...

.PHONY: build test