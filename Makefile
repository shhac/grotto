.PHONY: build run test app clean

# Fast dev build (bare binary)
build:
	go build -o grotto ./cmd/grotto

run:
	go run ./cmd/grotto

test:
	go test ./...

# Build a macOS .app bundle for local testing (shows icon in Dock)
app: build
	@./scripts/mkapp.sh Grotto.app grotto icon.png dev
	@echo "Built Grotto.app — run with: open Grotto.app"

clean:
	rm -rf grotto Grotto.app
