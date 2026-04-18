# agentlens Makefile
# ─────────────────────────────────────────────────────────────────────────────
#
#   make build       — compile the binary
#   make test        — go test ./...
#   make lint        — golangci-lint run
#   make check       — build + test + lint (pre-commit gate)
#   make sandbox     — reset run/, copy canonical sessions, launch agentlens
#   make reset       — wipe run/ only (no launch)
#   make clean       — remove binary and run/

BINARY     := ./agentlens
SESSIONS   := sandbox/sessions
RUN        := sandbox/run

.PHONY: build test lint check sandbox reset clean

# ── build ─────────────────────────────────────────────────────────────────────
build:
	go build -o $(BINARY) ./cmd/agentlens

# ── test ──────────────────────────────────────────────────────────────────────
test:
	go test ./...

# ── lint ──────────────────────────────────────────────────────────────────────
lint:
	golangci-lint run

# ── check (full pre-commit gate) ─────────────────────────────────────────────
check: build test lint

# ── reset: wipe the run directory ────────────────────────────────────────────
reset:
	@echo "→ wiping $(RUN)/"
	@rm -rf $(RUN)
	@echo "→ done"

# ── sandbox: reset + copy canonical sessions + launch ────────────────────────
#
# Layout:
#   sandbox/sessions/   — committed source-of-truth; never modified by the app
#   sandbox/run/        — fresh working copy; wiped and rebuilt on each `make sandbox`
#
# agentlens points at sandbox/run/ via --dir, so edits/backups happen there.
# The originals in sandbox/sessions/ are untouched.
sandbox: build reset
	@echo "→ copying canonical sessions → $(RUN)/"
	@cp -r $(SESSIONS)/. $(RUN)/
	@echo "→ sessions ready:"
	@find $(RUN) -name "*.jsonl" | while read f; do \
		proj=$$(basename $$(dirname $$f)); \
		size=$$(du -sh $$f | cut -f1); \
		echo "   $$size  $$proj/$$(basename $$f)"; \
	done
	@echo ""
	@echo "→ launching agentlens (--dir $(RUN)) ..."
	@$(BINARY) --dir $(RUN)

# ── clean ─────────────────────────────────────────────────────────────────────
clean: reset
	@echo "→ removing binary"
	@rm -f $(BINARY)
