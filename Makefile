all: test build

test:
	go test ./...

build:
	build_version=$$(git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD) && \
	if [ -n "$$(git status --porcelain)" ]; then build_version="$$build_version-dirty"; fi && \
	go build -ldflags="-w -s -X main.version=$${build_version#v}" -o bin/imgserv-linux-amd64
	sha256sum bin/imgserv-linux-amd64 > bin/imgserv.linux-amd64.sha256sum
