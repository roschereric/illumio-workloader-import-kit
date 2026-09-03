# umwl-tui — build, test and release helpers.
# Requires Go 1.24+. First build needs network access to fetch the Bubble Tea modules.
BIN      := umwl-tui
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

.PHONY: build tidy test vet fmt dist clean security

tidy:
	go mod tidy

build: tidy
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/umwl-tui

test: tidy
	go test ./...

vet:
	gofmt -l . && go vet ./...

fmt:
	gofmt -w .

# Static analysis + vulnerability scan (installs the tools if missing).
security:
	go run github.com/securego/gosec/v2/cmd/gosec@latest ./... || true
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

dist: tidy
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-$$os-$$arch ./cmd/umwl-tui; \
	done
	@cd dist && shasum -a 256 $(BIN)-* > SHA256SUMS && cat SHA256SUMS

clean:
	rm -rf dist $(BIN) runs
