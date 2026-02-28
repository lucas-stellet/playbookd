# playbookd

**Procedural memory for AI agents.** playbookd is a standalone Go library that gives agents the ability to learn how to perform tasks and improve over time.

Instead of rediscovering solutions on every run, an agent using playbookd can find existing playbooks for a task, follow their steps, record the outcome, and learn from what worked — creating a persistent, evolving knowledge base of procedures.

## Features

- **Hybrid search**: BM25 full-text search via [Bleve](https://blevesearch.com/) combined with cosine vector search via [FAISS](https://faiss.ai/) (optional, requires `-tags vectors`)
- **Composite scoring**: blend text relevance with Wilson confidence to surface battle-tested playbooks — `ConfidenceWeight` controls the mix
- **Contrastive search**: split results into proven (high confidence) and failed (low confidence) groups, so agents know what to follow and what to avoid
- **LLM context formatting**: render contrastive results as structured Markdown ready for in-context learning — inject directly into an agent's prompt
- **Wilson confidence scoring**: prevents a playbook with 1 success from outranking one with 95/100 — confidence is statistically grounded
- **Execution recording and reflection**: agents record step-by-step results; reflections are distilled into lessons that improve the playbook over time
- **Embedding providers**: built-in support for [OpenAI](https://platform.openai.com/), [Google Gemini](https://ai.google.dev/), and [Ollama](https://ollama.com/) — or bring your own `EmbeddingFunc`
- **CLI tool**: inspect, search, prune, and reindex your playbook store from the command line

## Installation

```sh
go get github.com/lucas-stellet/playbookd
```

## Usage

### Where to initialize

When integrating with a multi-agent system, place the `.playbookd.toml` and data directory at the **project root** (recommended). This way all agents share the same playbook store — one agent learns a procedure and others can find and reuse it.

```
my-project/
  .playbookd.toml
  playbooks/          # shared procedural memory
    playbooks/
    executions/
    index/
```

If your agents handle very different domains and shouldn't share memory, use a separate `DataDir` per agent:

```go
mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir: fmt.Sprintf("./workspaces/%s/playbooks", agentID),
})
```

### Creating a manager

The `PlaybookManager` is the main entry point. It coordinates storage, indexing, and embeddings.

**Without embeddings (BM25 keyword search only):**

```go
mgr, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir: "./playbooks",
})
if err != nil {
    log.Fatal(err)
}
defer mgr.Close()
```

This is the simplest setup — no API keys, no external dependencies. Search uses BM25 full-text matching, which works well for keyword-based queries.

**With embeddings (hybrid BM25 + vector search):**

```go
import "github.com/lucas-stellet/playbookd/embed"

embedFn := embed.Google(embed.GoogleConfig{
    APIKey: os.Getenv("GOOGLE_API_KEY"),
})

mgr, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: embedFn,
    EmbedDims: 768,
})
```

With embeddings, search understands semantic similarity — a query for "ship code to production" finds a playbook named "Deploy to production" even without matching keywords. Requires building with `-tags vectors` for full hybrid search (see [Build Tags](#build-tags)).

### Creating a playbook

```go
ctx := context.Background()

pb := &playbookd.Playbook{
    Name:        "Deploy to production",
    Description: "Steps to safely deploy a Go service to production",
    Category:    "deployment",
    Tags:        []string{"go", "deploy", "production"},
    Steps: []playbookd.Step{
        {Order: 1, Action: "Run test suite", Expected: "All tests pass"},
        {Order: 2, Action: "Build Docker image", Tool: "docker", ToolArgs: map[string]any{"cmd": "build"}},
        {Order: 3, Action: "Push image to registry"},
        {Order: 4, Action: "Apply Kubernetes manifests", Expected: "Rollout successful"},
    },
}

if err := mgr.Create(ctx, pb); err != nil {
    log.Fatal(err)
}
fmt.Printf("Created: %s (id: %s)\n", pb.Name, pb.ID)
```

The manager assigns an ID, generates a slug, computes the embedding (if configured), saves to disk, and indexes for search.

### Searching for playbooks

```go
results, err := mgr.Search(ctx, playbookd.SearchQuery{
    Text:  "deploy go service",
    Limit: 5,
})
if err != nil {
    log.Fatal(err)
}

for _, r := range results {
    fmt.Printf("[%.2f] %s (confidence: %.0f%%)\n",
        r.Score, r.Playbook.Name, r.Playbook.Confidence*100)
}
```

You can filter by category:

```go
results, _ := mgr.Search(ctx, playbookd.SearchQuery{
    Text:     "deploy",
    Category: "deployment",
    MinScore: 0.3,
})
```

#### Composite scoring

By default, results are ranked purely by text relevance. Set `ConfidenceWeight` to blend in the playbook's Wilson confidence score, so battle-tested playbooks rank higher:

```go
results, _ := mgr.Search(ctx, playbookd.SearchQuery{
    Text:             "deploy go service",
    ConfidenceWeight: 0.3, // 0=text only, 1=confidence only
})
```

The final score is computed as `(1 - weight) * normalizedTextScore + weight * confidence`. Text scores are min-max normalized to [0,1] before blending. A weight of 0 (the default) preserves the original ranking — existing code is unaffected.

### Contrastive search

Standard search returns a flat ranked list. Contrastive search goes further: it splits results into **proven** (high confidence) and **failed** (low confidence) groups, giving agents clear signal on what to follow and what to avoid.

```go
cr, err := mgr.SearchWithContext(ctx, playbookd.ContrastiveQuery{
    SearchQuery: playbookd.SearchQuery{
        Text:  "deploy go service",
        Limit: 5,
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Proven approaches: %d\n", len(cr.Positive))
fmt.Printf("Failed approaches: %d\n", len(cr.Negative))
```

The split is based on the playbook's Wilson confidence score (not the blended search score):
- **Positive**: confidence >= 0.5 (default, configurable via `PositiveMinConfidence`)
- **Negative**: confidence <= 0.3 (default, configurable via `NegativeMaxConfidence`)
- **Neutral**: everything in between (only included when `IncludeNeutral: true`)

Custom thresholds:

```go
cr, _ := mgr.SearchWithContext(ctx, playbookd.ContrastiveQuery{
    SearchQuery:           playbookd.SearchQuery{Text: "deploy"},
    PositiveMinConfidence: 0.7,  // stricter: only highly proven playbooks
    NegativeMaxConfidence: 0.2,  // stricter: only clearly failed playbooks
    IncludeNeutral:        true, // also return the middle ground
})
```

Internally, `SearchWithContext` searches with 3x the requested limit and `MinScore: 0` to capture low-quality matches that belong in the negative group, then caps each group to the original limit.

### Formatting results for LLM context

`FormatForContext` renders contrastive search results as structured Markdown that you can inject directly into an LLM's prompt for in-context learning:

```go
cr, _ := mgr.SearchWithContext(ctx, playbookd.ContrastiveQuery{
    SearchQuery: playbookd.SearchQuery{Text: "deploy go service"},
})

prompt := playbookd.FormatForContext(cr)
fmt.Println(prompt)
```

Output:

```markdown
## Playbook Context: deploy go service

### Proven Approaches (Follow These)

**1. Safe Production Deploy** (confidence: 85%, executions: 20)

Steps:
  1. Run test suite
  2. Build Docker image
  3. Deploy to staging
  4. Run smoke tests
  5. Promote to production

Lessons learned:
  - Always run smoke tests before promoting

### Failed Approaches (Avoid These)

**1. Quick Deploy** (confidence: 5%, failure rate: 90%)

What failed:
  - Skipping tests caused repeated rollbacks

---
Follow the proven approaches. Avoid the patterns described in failed approaches.
```

This output is designed to be concatenated into a system prompt or user message. The agent receives structured procedural memory — what works, what doesn't, and why — without needing to interpret raw scores.

`FormatForContext` handles edge cases: returns `""` for nil input, and `"No relevant playbooks found for: <query>"` when both groups are empty.

### Recording an execution

After an agent follows a playbook, record the outcome:

```go
rec := &playbookd.ExecutionRecord{
    PlaybookID:  pb.ID,
    PlaybookVer: pb.Version,
    AgentID:     "agent-1",
    StartedAt:   startTime,
    CompletedAt: time.Now(),
    Outcome:     playbookd.OutcomeSuccess,
    TaskContext:  "User requested production deploy of api-server v2.1",
    StepResults: []playbookd.StepResult{
        {StepOrder: 1, Outcome: playbookd.OutcomeSuccess, Output: "42 tests passed"},
        {StepOrder: 2, Outcome: playbookd.OutcomeSuccess, Duration: "45s"},
        {StepOrder: 3, Outcome: playbookd.OutcomeSuccess},
        {StepOrder: 4, Outcome: playbookd.OutcomeSuccess},
    },
}

if err := mgr.RecordExecution(ctx, rec); err != nil {
    log.Fatal(err)
}
```

Recording an execution automatically:
- Updates `SuccessCount`/`FailureCount` on the playbook
- Recalculates the Wilson confidence score

Available outcomes: `OutcomeSuccess`, `OutcomePartial` (counts as success for stats), `OutcomeFailure`.

### Learning from reflections

Reflections let agents capture what worked, what failed, and how to improve. When `AutoReflect` is enabled, improvements are automatically added as lessons to the playbook:

```go
mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:     "./playbooks",
    AutoReflect: true,
})

rec := &playbookd.ExecutionRecord{
    PlaybookID: pb.ID,
    Outcome:    playbookd.OutcomeSuccess,
    Reflection: &playbookd.Reflection{
        WhatWorked:   []string{"Layer caching made the Docker build fast"},
        WhatFailed:   []string{"Health check took 30s to pass"},
        Improvements: []string{"Add a readiness probe timeout of 60s"},
        ShouldUpdate: true,
    },
}

mgr.RecordExecution(ctx, rec)
// The improvement is now a Lesson on the playbook, embedded and re-indexed.
```

You can also apply reflections manually:

```go
mgr.ApplyReflection(ctx, pb.ID, &playbookd.Reflection{
    Improvements: []string{"Run smoke tests after deploy"},
    ShouldUpdate: true,
})
```

### Retrieving and listing playbooks

```go
// Get a playbook by ID
pb, err := mgr.Get(ctx, "playbook-uuid-here")

// List all playbooks
all, _ := mgr.List(ctx, playbookd.ListFilter{})

// List playbooks in a category
deploys, _ := mgr.List(ctx, playbookd.ListFilter{
    Category: "deployment",
})

// List playbooks with specific tags
tagged, _ := mgr.List(ctx, playbookd.ListFilter{
    Tags: []string{"go", "production"},
})
```

### Updating and deleting playbooks

```go
// Update a playbook (increments version, re-embeds, re-indexes)
pb.Steps = append(pb.Steps, playbookd.Step{
    Order: 5, Action: "Run smoke tests", Expected: "Health endpoint returns 200",
})
mgr.Update(ctx, pb)

// Delete a playbook (removes from store and index)
mgr.Delete(ctx, pb.ID)
```

### Listing executions

```go
// Get the last 10 executions for a playbook
execs, _ := mgr.ListExecutions(ctx, pb.ID, 10)
for _, e := range execs {
    fmt.Printf("%s — %s (%s)\n", e.ID, e.Outcome, e.CompletedAt.Format(time.RFC3339))
}
```

### Pruning stale playbooks

Prune archives playbooks that are stale or have low confidence:

```go
// Dry run — see what would be archived
result, _ := mgr.Prune(ctx, playbookd.PruneOptions{DryRun: true})
fmt.Printf("Would archive: %v\n", result.Archived)

// Archive for real
result, _ = mgr.Prune(ctx, playbookd.PruneOptions{})
fmt.Printf("Archived: %v\n", result.Archived)

// Custom thresholds
result, _ = mgr.Prune(ctx, playbookd.PruneOptions{
    MaxAge:        60 * 24 * time.Hour, // 60 days
    MinConfidence: 0.5,
})
```

### Aggregate statistics

```go
stats, _ := mgr.Stats(ctx)
fmt.Printf("Total playbooks: %d\n", stats.TotalPlaybooks)
fmt.Printf("Total executions: %d\n", stats.TotalExecs)
fmt.Printf("Avg confidence: %.0f%%\n", stats.AvgConfidence*100)
fmt.Printf("Archived: %d\n", stats.TotalArchived)
fmt.Printf("By category: %v\n", stats.ByCategory)
```

### Rebuilding the index

If you manually edit playbook JSON files or recover from index corruption:

```go
mgr.Reindex(ctx)
```

### Manager configuration reference

```go
playbookd.ManagerConfig{
    DataDir:       "./playbooks",          // Root directory for all data (required)
    EmbedFunc:     embedFn,                // Embedding function (nil = BM25 only)
    EmbedDims:     768,                    // Embedding dimensions (0 = BM25 only)
    AutoReflect:   true,                   // Auto-apply reflections as lessons
    MaxAge:        90 * 24 * time.Hour,    // Max age before prunable (default: 90 days)
    MinConfidence: 0.3,                    // Min confidence for pruning (default: 0.3)
    Logger:        slog.Default(),         // Structured logger (default: slog.Default())
}
```

## Configuration

playbookd can be configured with a `.playbookd.toml` file. Generate one with:

```sh
playbookd init
```

Or pre-select a provider:

```sh
playbookd init -provider google
```

Example configuration:

```toml
[embedding]
provider = "google"
model = "gemini-embedding-001"
api_key = "${GOOGLE_API_KEY}"
url = "https://generativelanguage.googleapis.com/v1beta"
dimensions = 768

[data]
dir = "./playbooks"

[manager]
auto_reflect = false
max_age = "90d"
min_confidence = 0.3
```

Supported providers:

| Provider | `provider` | Default model | Dimensions |
|----------|-----------|---------------|------------|
| Google Gemini | `"google"` | `gemini-embedding-001` | 768–3072 |
| OpenAI | `"openai"` | `text-embedding-3-small` | 1536 |
| Ollama (local) | `"ollama"` | `nomic-embed-text-v2-moe` | 384 |
| None | `"noop"` | — | — |

The `api_key` field supports environment variable expansion: `"${GOOGLE_API_KEY}"` is replaced with the value of `GOOGLE_API_KEY` at load time.

Environment variables override the config file: `PLAYBOOKD_DATA` takes precedence over `[data] dir`.

## Embedding providers

Each provider requires an API key or a running local server. Get your API key from:

- **Google Gemini**: [Google AI Studio](https://aistudio.google.com/apikey) (free tier available)
- **OpenAI**: [OpenAI Platform](https://platform.openai.com/api-keys)
- **Ollama**: No key needed — run locally with `ollama serve`

**Google Gemini**

```go
import "github.com/lucas-stellet/playbookd/embed"

embedFn := embed.Google(embed.GoogleConfig{
    APIKey: os.Getenv("GOOGLE_API_KEY"),
    // Model defaults to "gemini-embedding-001"
})

mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: embedFn,
    EmbedDims: 768,
})
```

**OpenAI**

```go
embedFn := embed.OpenAI(embed.OpenAIConfig{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    // Model defaults to "text-embedding-3-small"
})

mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: embedFn,
    EmbedDims: 1536,
})
```

**Ollama (local)**

```go
embedFn := embed.Ollama(embed.OllamaConfig{
    // URL defaults to "http://localhost:11434"
    // Model defaults to "nomic-embed-text-v2-moe"
})

mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: embedFn,
    EmbedDims: 384,
})
```

**Custom provider**

Implement the `embed.EmbeddingFunc` signature:

```go
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)
```

```go
myEmbed := func(ctx context.Context, text string) ([]float32, error) {
    // Call your embedding API here
    return vector, nil
}

mgr, _ := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: myEmbed,
    EmbedDims: 256, // match your model's output dimensions
})
```

## Enabling vector search (FAISS)

By default, embeddings are stored but search uses BM25 only. To enable hybrid BM25 + cosine vector search, build with the `vectors` tag:

```sh
CGO_ENABLED=1 go build -tags vectors ./...
```

This requires FAISS to be installed (see [FAISS Installation Guide](#faiss-installation-guide)).

## CLI

Install the CLI:

```sh
go install github.com/lucas-stellet/playbookd/cmd/playbookd@latest
```

By default the CLI looks for data in `./playbooks`. Override with the `PLAYBOOKD_DATA` environment variable:

```sh
export PLAYBOOKD_DATA=/var/lib/playbookd
```

If a `.playbookd.toml` file exists in the current directory, the CLI uses it for configuration (embedding provider, data dir, manager settings). The `PLAYBOOKD_DATA` env var still takes precedence over the TOML `[data] dir`.

### Commands

**Initialize configuration**

```sh
# Generate a default .playbookd.toml
playbookd init

# Pre-select a provider
playbookd init -provider google

# Overwrite existing config
playbookd init -force
```

**List playbooks**

```sh
# List all playbooks
playbookd list

# Include archived playbooks
playbookd list -archived

# Filter by category
playbookd list -category deployment
```

**Search playbooks**

```sh
playbookd search "deploy go service to kubernetes"
```

**Get a specific playbook**

```sh
playbookd get <id>
```

**Show aggregate statistics**

```sh
playbookd stats
```

**Prune stale playbooks**

Archives stale playbooks with low confidence or those not used within the configured `MaxAge`:

```sh
# Dry run — show what would be archived
playbookd prune -dry-run

# Archive stale playbooks
playbookd prune
```

**Rebuild the search index**

Use after manually editing playbook files or recovering from index corruption:

```sh
playbookd reindex
```

## Build Tags

playbookd has two build modes:

| Mode | Tag | Search | FAISS required |
|------|-----|--------|----------------|
| BM25 only (default) | _(none)_ | Full-text keyword search via Bleve | No |
| Hybrid | `-tags vectors` | BM25 + cosine vector search via FAISS | Yes |

The default build has no CGO dependency and works anywhere Go runs. The hybrid build requires FAISS to be installed and CGO enabled.

```sh
# Default build — no CGO, no FAISS
go build ./...

# Hybrid build — requires FAISS installed and CGO
CGO_ENABLED=1 go build -tags vectors ./...
```

## FAISS Installation Guide

FAISS is only required when building with `-tags vectors`. Skip this section if you use the default BM25-only build.

---

### macOS

**Option 1: Homebrew (recommended)**

```sh
brew install faiss
```

Homebrew installs the shared library and headers. CGO will find them automatically.

**Option 2: Build from source**

```sh
# Prerequisites
brew install cmake libomp

# Clone and build
git clone https://github.com/facebookresearch/faiss.git
cd faiss
cmake -B build \
  -DFAISS_ENABLE_C_API=ON \
  -DBUILD_SHARED_LIBS=ON \
  -DFAISS_ENABLE_GPU=OFF \
  -DBUILD_TESTING=OFF \
  -DCMAKE_BUILD_TYPE=Release
cmake --build build --parallel
sudo cmake --install build
```

---

### Linux (Ubuntu / Debian)

**Option 1: Build from source (recommended for C API support)**

```sh
# Prerequisites
sudo apt update
sudo apt install -y build-essential cmake libopenblas-dev

# Clone FAISS
git clone https://github.com/facebookresearch/faiss.git
cd faiss

# Configure
cmake -B build \
  -DFAISS_ENABLE_C_API=ON \
  -DBUILD_SHARED_LIBS=ON \
  -DFAISS_ENABLE_GPU=OFF \
  -DBUILD_TESTING=OFF \
  -DCMAKE_BUILD_TYPE=Release \
  .

# Build and install
make -C build -j$(nproc)
sudo make -C build install
sudo ldconfig
```

This installs headers to `/usr/local/include/faiss` and the shared library to `/usr/local/lib`.

**Option 2: conda**

```sh
conda install -c pytorch faiss-cpu
```

> Note: the conda package does not always include the C API headers needed for CGO. If you encounter linking errors, prefer building from source.

---

### Windows

Native Windows builds are possible but involve extra steps. **WSL2 is strongly recommended** as it gives you a full Linux environment where the Linux instructions above work without modification.

**WSL2 (recommended)**

1. Enable WSL2 and install Ubuntu from the Microsoft Store
2. Open a WSL2 terminal and follow the [Linux instructions](#linux-ubuntu--debian) above
3. Run your Go builds inside WSL2

**Native Windows (advanced)**

Requirements:
- [Visual Studio Build Tools](https://visualstudio.microsoft.com/downloads/#build-tools-for-visual-studio-2022) with the "Desktop development with C++" workload
- [CMake](https://cmake.org/download/) (add to PATH during install)
- [OpenBLAS for Windows](https://github.com/OpenMathLib/OpenBLAS/releases) or Intel MKL

```bat
git clone https://github.com/facebookresearch/faiss.git
cd faiss

cmake -B build -G "Visual Studio 17 2022" -A x64 ^
  -DFAISS_ENABLE_C_API=ON ^
  -DBUILD_SHARED_LIBS=ON ^
  -DFAISS_ENABLE_GPU=OFF ^
  -DBUILD_TESTING=OFF

cmake --build build --config Release
cmake --install build
```

Add the install `bin/` directory to your `PATH` so the DLL is found at runtime.

---

### Verifying the installation

After installing FAISS, verify the Go build succeeds:

```sh
CGO_ENABLED=1 go build -tags vectors ./...
```

If you see linker errors about missing `faiss_c`, ensure the shared library is in your system's library search path (`/usr/local/lib` on Linux, or the Homebrew prefix on macOS).

---

## Architecture

```
Agent
  │
  ▼
PlaybookManager
  ├── Search(query) ──────────────────────────────┐
  │     ├── EmbeddingFunc (optional)              │
  │     ├── Indexer (Bleve + FAISS)               │
  │     │     ├── BM25 text search                │
  │     │     └── cosine vector search (-tags vectors)
  │     └── Composite scoring (ConfidenceWeight)  │
  │                                               │
  ├── SearchWithContext(query) ◄── contrastive    │
  │     ├── splits into Positive / Negative       │
  │     └── FormatForContext() → Markdown for LLM │
  │                                               │
  ├── Create / Update / Delete ──► Store (JSON files)
  │                                               │
  └── RecordExecution                             │
        ├── updates SuccessCount / FailureCount   │
        ├── recalculates Wilson confidence ◄──────┘
        └── ApplyReflection → adds Lessons → Update
```

### Data layout on disk

```
<DataDir>/
  playbooks/
    <id>.json          # Playbook data (with embedding)
  executions/
    <playbook-id>/
      <exec-id>.json   # Execution records
  index/               # Bleve index (BM25 + optional vector index)
```

## License

MIT
