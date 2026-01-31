.PHONY: all build frontend dev clean

all: deps frontend build

deps:
	cd web && npm install
	go mod download

frontend:
	cd web && npm run build
	rm -rf embedded/frontend
	cp -r web/out embedded/frontend

build: frontend
	go build -o bin/jellywatchd ./cmd/jellywatchd
	go build -o bin/jellywatch ./cmd/jellywatch

dev:
	go run ./cmd/jellywatchd --health-addr=:8686 &
	cd web && npm run dev

clean:
	rm -rf bin/ web/out web/.next embedded/frontend
