all: test build

test:
	go test

build:
	go build -ldflags="-w -s" -o bin/imgserv-linux-amd64
	sha256sum bin/imgserv-linux-amd64 > bin/imgserv.linux-amd64.sha256sum