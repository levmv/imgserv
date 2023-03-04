all: test build

test:
	go test

build:
	go build -ldflags="-w -s" -o bin/go-resizer-linux-amd64
	sha256sum bin/go-resizer-linux-amd64 > bin/go-resizer.linux-amd64.sha256sum