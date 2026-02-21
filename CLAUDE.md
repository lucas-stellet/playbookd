# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

playbookd is a Go library that provides procedural memory for AI agents. Agents can create, search, execute, and learn from playbooks — persistent, versioned procedures with confidence-scored outcomes.

## Build Commands

```sh
# Default build (BM25-only, no CGO required)
go build ./...

# Hybrid build with FAISS vector search (requires FAISS installed + CGO)
CGO_ENABLED=1 go build -tags vectors ./...

# Run all tests
go test ./...

# Run a single test
go test -run TestManagerCreate ./...

# Run tests in a subpackage
go test ./embed/...
```

## Build Tags

The codebase uses Go build tags to conditionally compile vector search support:
- **Default (no tag)**: BM25-only search via Bleve. No CGO dependency.
- **`-tags vectors`**: Enables hybrid BM25 + FAISS cosine vector search. Requires CGO and FAISS.

The split is in `indexer_vectors.go` (FAISS-enabled) and `indexer_no_vectors.go` (stubs that fall back to BM25). Both files implement the same three methods (`addVectorMapping`, `buildVectorRequest`, `buildHybridRequest`).

## Architecture

The library is a single Go package (`playbookd`) at the repo root with one subpackage (`embed/`) and a CLI (`cmd/playbookd/`).

**Core flow**: Agent → `PlaybookManager` → `Store` (JSON files on disk) + `Indexer` (Bleve search index)

Key interfaces and their implementations:
- **`Store`** (interface in `store.go`) → `FileStore`: persists playbooks and execution records as JSON files using atomic writes (temp file + rename).
- **`Indexer`** (interface in `indexer.go`) → `BleveIndexer`: BM25 full-text search with optional FAISS vector search. Indexes fields: name, description, tags, steps, lessons, category, status, confidence, success_rate.
- **`PlaybookManager`** (`manager.go`): orchestrates Store + Indexer + embedding. All CRUD operations go through the manager, which keeps store and index in sync.

**Embedding providers** (`embed/` package):
- `embed.Noop()` — returns nil embeddings (BM25-only mode)
- `embed.Ollama(cfg)` — calls Ollama API (default model: `nomic-embed-text-v2-moe`)
- `embed.OpenAI(cfg)` — calls OpenAI-compatible API (default model: `text-embedding-3-small`)

**Playbook lifecycle**: `draft` → `active` (auto-promoted after 3 successes) → `deprecated` (auto-deprecated when success rate < 30% with 5+ executions) → `archived` (via prune)

**Wilson confidence scoring** (`playbook.go:WilsonConfidence`): uses the Wilson score interval lower bound at 95% CI to rank playbooks, preventing low-sample-size playbooks from outranking well-tested ones.

## Data Layout on Disk

```
<DataDir>/
  playbooks/<id>.json       # Playbook data
  executions/<pb-id>/<exec-id>.json  # Execution records
  index/                    # Bleve index files
```

## CLI

The CLI is at `cmd/playbookd/`. It reads `PLAYBOOKD_DATA` env var (default: `./playbooks`) for the data directory. Commands: list, search, get, stats, prune, reindex.
