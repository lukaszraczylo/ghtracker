.PHONY: ui build test lint dev

ui:
	pnpm install --frozen-lockfile
	pnpm build

build: ui
	go build -o ghtracker .

test:
	pnpm test
	go test -race ./...

lint:
	pnpm lint
	pnpm typecheck
	golangci-lint run ./...

# Run the Go server (config.yaml) and the Vite dev server with hot reload; open http://localhost:5173.
dev:
	go run . -config config.yaml & pnpm dev
