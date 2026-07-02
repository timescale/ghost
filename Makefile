.PHONY: install build test docs

# Linker flags to ignore protobuf registration conflicts. Both the sqlc CLI and
# the sqlc plugin SDK (linked in via internal/mcp/query) register the same
# protobuf definitions; this tells the protobuf library to silently ignore the
# duplicate registration. Without it, every built binary — including test
# binaries and `go run` — panics at startup, so all builds must go through
# these targets (or pass the flag explicitly).
# See https://protobuf.dev/reference/go/faq#namespace-conflict
LDFLAGS := -X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=ignore

install:
	go install -ldflags="$(LDFLAGS)" ./...

build:
	go build -ldflags="$(LDFLAGS)" ./...

# Extra `go test` arguments (e.g. -race, coverage flags) can be passed via
# TESTFLAGS: make test TESTFLAGS='-race -coverprofile=coverage.out'
test:
	go test -ldflags="$(LDFLAGS)" $(TESTFLAGS) ./...

# Regenerate the CLI reference docs and tutorial docs.
docs:
	go run -ldflags="$(LDFLAGS)" ./cmd/generate-docs -clean
	go run -ldflags="$(LDFLAGS)" ./cmd/generate-tutorial-docs
