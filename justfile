binary    := "./seshr"
sessions  := "sandbox/sessions"
run       := "sandbox/run"

build:
    go build -o {{ binary }} ./cmd/seshr

test:
    go test ./...

lint:
    golangci-lint run

check: build test lint

reset:
    echo "→ wiping {{ run }}/"
    rm -rf {{ run }}
    echo "→ done"

sandbox: build reset
    #!/usr/bin/env bash
    echo "→ copying canonical sessions → {{ run }}/"
    cp -r {{ sessions }}/. {{ run }}/
    echo "→ sessions ready:"
    find {{ run }} -name "*.jsonl" | while read f; do
        proj=$(basename $(dirname "$f"))
        size=$(du -sh "$f" | cut -f1)
        echo "   $size  $proj/$(basename "$f")"
    done
    echo ""
    echo "→ launching seshr (--dir {{ run }}) ..."
    {{ binary }} --dir {{ run }}

clean: reset
    echo "→ removing binary"
    rm -f {{ binary }}
