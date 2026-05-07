.PHONY: build build-all build-linux build-darwin clean

build:
	go build -o model-mapper .

build-all: build-darwin build-linux

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/model-mapper-darwin-arm64 .

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/model-mapper-linux-amd64 .

clean:
	rm -f model-mapper
	rm -rf dist/
