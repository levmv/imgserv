all: test build

test:
	go test

build:
	go build -ldflags="-w -s"