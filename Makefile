.PHONY: all build frontend dev clean check-frontend

all: deps frontend build

deps:
	cd web && npm install
	go mod download

frontend:
	cd web && npm run build
	rm -rf embedded/frontend
	cp -r web/out embedded/frontend

# Validates that frontend is built with real content, not placeholder
check-frontend:
	@if [ ! -f embedded/frontend/index.html ]; then \
		echo "ERROR: embedded/frontend/ not built. Run 'make frontend' first."; \
		exit 1; \
	fi
	@if grep -q '<body>plex2jellyfin</body>' embedded/frontend/index.html 2>/dev/null; then \
		echo "ERROR: embedded/frontend/index.html is placeholder content. Run 'make frontend'."; \
		exit 1; \
	fi
	@echo "Frontend check passed."

build: check-frontend
	go build -o bin/plex2jellyfin-daemon ./cmd/plex2jellyfin-daemon
	go build -o bin/plex2jellyfin ./cmd/plex2jellyfin
	go build -o bin/plex2jellyfin-web ./cmd/plex2jellyfin-web

dev:
	go run ./cmd/plex2jellyfin-daemon --health-addr=:8686 &
	cd web && npm run dev

clean:
	rm -rf bin/ web/out web/.next embedded/frontend
