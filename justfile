binary    := "./seshr"

build:
    go build -o {{ binary }} ./cmd/seshr

test:
    go test ./...

lint:
    golangci-lint run

check: build test lint

sandbox: build
    {{ binary }} --dir {{ justfile_directory() }}/sandbox/claude-sessions --opencode-db {{ justfile_directory() }}/sandbox/opencode.db --no-live

clean:
    echo "→ removing binary"
    rm -f {{ binary }}
