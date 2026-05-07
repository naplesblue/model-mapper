.PHONY: build build-linux clean

build:
	go build -o model-mapper .

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o model-mapper-linux-amd64 .

clean:
	rm -f model-mapper model-mapper-linux-amd64
