.PHONY: generate build run

generate:
	go generate ./internal/api

build: generate
	go build -o bin/server ./cmd/server

run: generate
	go run ./cmd/server