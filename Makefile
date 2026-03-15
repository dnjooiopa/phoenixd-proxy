-include .env
export


test:
	go test -v ./...

run:
	go run main.go

dev:
	goreload .
