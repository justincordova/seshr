binary    := "./seshr"

build:
    go build -o {{ binary }} ./cmd/seshr

test:
    go test ./...

lint:
    golangci-lint run

check: build test lint

clean:
    echo "→ removing binary"
    rm -f {{ binary }}
