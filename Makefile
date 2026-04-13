.PHONY: build dev frontend backend clean test

# Default target
build: backend

# Build frontend (requires Node.js), copy to embed location
frontend:
	cd web && npm install && npm run build
	rm -rf internal/frontend/build
	cp -r web/build internal/frontend/build

# Build Go binary (embeds frontend)
backend:
	go build -o bin/fine-print ./cmd/fine-print

# Build everything
all: frontend backend

# Run in development mode
dev:
	FINEPRINT_DEV=1 FINEPRINT_PORT=8080 go run ./cmd/fine-print -dev

# Run frontend dev server
dev-frontend:
	cd web && npm run dev -- --port 5173

# Run backend with frontend proxy
dev-backend:
	FINEPRINT_DEV=1 FINEPRINT_PORT=8080 FINEPRINT_FRONTEND_PROXY=http://localhost:5173 go run ./cmd/fine-print -dev

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf web/build/
	rm -rf internal/frontend/build
	mkdir -p internal/frontend/build
	echo '<!DOCTYPE html><html><body>Frontend not built</body></html>' > internal/frontend/build/index.html

# Cross-compile for Raspberry Pi
build-pi:
	GOOS=linux GOARCH=arm64 go build -o bin/fine-print-linux-arm64 ./cmd/fine-print

# Cross-compile for Linux x86_64
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/fine-print-linux-amd64 ./cmd/fine-print

# Install dependencies
deps:
	go mod tidy
	cd web && npm install
