SHELL := /bin/bash

all: build test
clean:
	rm -fv bin/*
build: clean
	go build -o bin/crontinuous
build-arch: clean
	GOOS=linux GOARCH=amd64 go build -o bin/crontinuous-linux-amd64
	GOOS=darwin GOARCH=amd64 go build -o bin/crontinuous-darwin-amd64
build-docker: build-arch
	docker build .
docker-build: build-docker
test:
	go test ./...
