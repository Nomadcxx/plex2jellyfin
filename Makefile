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
	@if grep -q '<body>jellywatch</body>' embedded/frontend/index.html 2>/dev/null; then \
		echo "ERROR: embedded/frontend/index.html is placeholder content. Run 'make frontend'."; \
		exit 1; \
	fi
	@echo "Frontend check passed."

build: check-frontend
	go build -o bin/jellywatchd ./cmd/jellywatchd
	go build -o bin/jellywatch ./cmd/jellywatch

dev:
	go run ./cmd/jellywatchd --health-addr=:8686 &
	cd web && npm run dev

clean:
	rm -rf bin/ web/out web/.next embedded/frontend
