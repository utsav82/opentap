-include .env
export

.PHONY: dev api worker dashboard test lint build-api build-worker

dev:
	docker compose -f docker-compose.dev.yml up -d

api:
	go run cmd/api/main.go

worker:
	go run cmd/worker/main.go

dashboard:
	cd dashboard && npm install && npm run dev

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./...

build-api:
	CGO_ENABLED=0 go build -o bin/api cmd/api/main.go

build-worker:
	CGO_ENABLED=0 go build -o bin/worker cmd/worker/main.go
