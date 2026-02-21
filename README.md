# playbookd

**Procedural memory for AI agents.** playbookd is a standalone Go library that gives agents the ability to learn how to perform tasks and improve over time.

Instead of rediscovering solutions on every run, an agent using playbookd can find existing playbooks for a task, follow their steps, record the outcome, and learn from what worked — creating a persistent, evolving knowledge base of procedures.

## Features

- **Hybrid search**: BM25 full-text search via [Bleve](https://blevesearch.com/) combined with cosine vector search via [FAISS](https://faiss.ai/) (optional, requires `-tags vectors`)
- **Wilson confidence scoring**: prevents a playbook with 1 success from outranking one with 95/100 — confidence is statistically grounded
- **Playbook lifecycle**: `draft` → `active` → `deprecated` → `archived`, with automatic promotion and deprecation based on execution outcomes
- **Execution recording and reflection**: agents record step-by-step results; reflections are distilled into lessons that improve the playbook over time
- **CLI tool**: inspect, search, prune, and reindex your playbook store from the command line

## Installation

```sh
go get github.com/lucas-stellet/playbookd
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/lucas-stellet/playbookd"
)

func main() {
    ctx := context.Background()

    // Create a manager (BM25-only mode, no FAISS required)
    mgr, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
        DataDir: "./playbooks",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer mgr.Close()

    // Create a playbook
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
    fmt.Printf("Created playbook: %s (%s)\n", pb.Name, pb.ID)

    // Search for a playbook
    results, err := mgr.Search(ctx, playbookd.SearchQuery{
        Text:  "deploy go service",
        Limit: 5,
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, r := range results {
        fmt.Printf("[%.2f] %s — %s\n", r.Score, r.Playbook.Name, r.Playbook.Status)
    }

    // Record an execution
    rec := &playbookd.ExecutionRecord{
        PlaybookID: pb.ID,
        AgentID:    "agent-1",
        Outcome:    playbookd.OutcomeSuccess,
        StepResults: []playbookd.StepResult{
            {StepOrder: 1, Outcome: playbookd.OutcomeSuccess},
            {StepOrder: 2, Outcome: playbookd.OutcomeSuccess},
            {StepOrder: 3, Outcome: playbookd.OutcomeSuccess},
            {StepOrder: 4, Outcome: playbookd.OutcomeSuccess},
        },
        Reflection: &playbookd.Reflection{
            WhatWorked:   []string{"Image build was fast due to layer caching"},
            WhatFailed:   []string{},
            Improvements: []string{"Add a health check step after rollout"},
            ShouldUpdate: true,
        },
    }

    if err := mgr.RecordExecution(ctx, rec); err != nil {
        log.Fatal(err)
    }
    fmt.Println("Execution recorded.")
}
```

### Enabling vector search (FAISS)

To use hybrid BM25 + vector search, provide an embedding function and build with `-tags vectors`:

```go
mgr, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
    DataDir:   "./playbooks",
    EmbedFunc: myEmbeddingFunc, // func(ctx, text) ([]float32, error)
    EmbedDims: 1536,            // match your model's output dimensions
})
```

```sh
CGO_ENABLED=1 go build -tags vectors ./...
```

## CLI

Install the CLI:

```sh
go install github.com/lucas-stellet/playbookd/cmd/playbookd@latest
```

By default the CLI looks for data in `./playbooks`. Override with the `PLAYBOOKD_DATA` environment variable:

```sh
export PLAYBOOKD_DATA=/var/lib/playbookd
```

### Commands

**List playbooks**

```sh
# List all playbooks
playbookd list

# Filter by status
playbookd list -status active

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

Archives deprecated playbooks and those not used within the configured `MaxAge`:

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
  │     └── Indexer (Bleve + FAISS)               │
  │           ├── BM25 text search                │
  │           └── cosine vector search (-tags vectors)
  │                                               │
  ├── Create / Update / Delete ──► Store (JSON files)
  │                                               │
  └── RecordExecution                             │
        ├── updates SuccessCount / FailureCount   │
        ├── recalculates Wilson confidence ◄──────┘
        ├── auto-promotes draft → active (≥3 successes)
        ├── auto-deprecates on failure rate > 70%
        └── ApplyReflection → adds Lessons → Update
```

### Playbook lifecycle

```
draft ──(3 successes)──► active ──(failure rate > 70%)──► deprecated
                                                               │
                         prune ◄────────────────────────────── ┘
                           │
                           ▼
                        archived
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

## Integration with picoclaw

playbookd is designed to plug into [picoclaw](https://github.com/sipeed/picoclaw) (or any Go-based AI agent). The integration touches 4 files in picoclaw:

### 1. Add dependency

```sh
cd picoclaw
go get github.com/lucas-stellet/playbookd
```

### 2. Config (`pkg/config/config.go`)

Add `PlaybookConfig` to the main `Config` struct:

```go
type Config struct {
    // ... existing fields ...
    Playbook PlaybookConfig `json:"playbook"`
}

type PlaybookConfig struct {
    Enabled            bool    `json:"enabled"`
    DataDir            string  `json:"data_dir"`             // default: "playbooks"
    EmbedModel         string  `json:"embed_model"`          // default: "nomic-embed-text-v2-moe"
    EmbedURL           string  `json:"embed_url"`            // default: "http://localhost:11434"
    AutoReflect        bool    `json:"auto_reflect"`         // default: true
    DetectionThreshold int     `json:"detection_threshold"`  // min tool calls to detect a task (default: 3)
}
```

Example `config.json`:

```json
{
  "playbook": {
    "enabled": true,
    "data_dir": "playbooks",
    "embed_model": "nomic-embed-text-v2-moe",
    "embed_url": "http://localhost:11434",
    "auto_reflect": true,
    "detection_threshold": 3
  }
}
```

### 3. Tools registration (`pkg/agent/loop.go`)

Register `playbook_search` and `playbook_record` as shared tools in `registerSharedTools`:

```go
import (
    "github.com/lucas-stellet/playbookd"
    "github.com/lucas-stellet/playbookd/embed"
)

// In NewAgentLoop or registerSharedTools:
if cfg.Playbook.Enabled {
    embedFn := embed.Ollama(embed.OllamaConfig{
        URL:   cfg.Playbook.EmbedURL,
        Model: cfg.Playbook.EmbedModel,
    })

    pbManager, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
        DataDir:     cfg.Playbook.DataDir,
        EmbedFunc:   embedFn,
        EmbedDims:   768, // nomic-embed-text-v2-moe
        AutoReflect: cfg.Playbook.AutoReflect,
    })
    if err != nil {
        logger.WarnCF("agent", "Failed to init playbook manager", map[string]any{"error": err.Error()})
    } else {
        // Register playbook tools for each agent
        for _, agentID := range registry.ListAgentIDs() {
            agent, ok := registry.GetAgent(agentID)
            if !ok {
                continue
            }
            agent.Tools.Register(NewPlaybookSearchTool(pbManager))
            agent.Tools.Register(NewPlaybookRecordTool(pbManager))
        }
    }
}
```

The tools follow picoclaw's `Tool` interface:

```go
// PlaybookSearchTool — agent calls this to find relevant playbooks
type PlaybookSearchTool struct {
    manager *playbookd.PlaybookManager
}

func (t *PlaybookSearchTool) Name() string        { return "playbook_search" }
func (t *PlaybookSearchTool) Description() string  { return "Search procedural memory for relevant playbooks" }
func (t *PlaybookSearchTool) Parameters() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": map[string]any{"type": "string", "description": "Natural language description of the task"},
            "limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
        },
        "required": []string{"query"},
    }
}
func (t *PlaybookSearchTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
    query, _ := args["query"].(string)
    limit := 5
    if l, ok := args["limit"].(float64); ok {
        limit = int(l)
    }
    results, err := t.manager.Search(ctx, playbookd.SearchQuery{
        Text:  query,
        Limit: limit,
    })
    if err != nil {
        return tools.ErrorResult(err.Error())
    }
    // Format results as JSON for the LLM
    data, _ := json.Marshal(results)
    return tools.SilentResult(string(data))
}
```

### 4. System prompt (`pkg/agent/context.go`)

Add a "Procedural Memory" section to `BuildSystemPrompt` that lists known playbooks so the agent is aware of its experience:

```go
func (cb *ContextBuilder) BuildSystemPrompt() string {
    parts := []string{}
    parts = append(parts, cb.getIdentity())

    // ... existing sections ...

    // Procedural Memory — list active playbooks
    if cb.playbookManager != nil {
        active := playbookd.StatusActive
        playbooks, _ := cb.playbookManager.List(context.Background(), playbookd.ListFilter{
            Status: &active,
            Limit:  10,
        })
        if len(playbooks) > 0 {
            var sb strings.Builder
            sb.WriteString("# Procedural Memory\n\n")
            sb.WriteString("You have learned procedures for the following tasks. ")
            sb.WriteString("Use `playbook_search` before starting multi-step tasks.\n\n")
            for _, pb := range playbooks {
                fmt.Fprintf(&sb, "- **%s** (confidence: %.0f%%, last used: %s)\n",
                    pb.Name, pb.Confidence*100, pb.LastUsedAt.Format("2006-01-02"))
            }
            parts = append(parts, sb.String())
        }
    }

    return strings.Join(parts, "\n\n---\n\n")
}
```

### 5. Automatic task detection (`pkg/agent/loop.go`)

In `runLLMIteration`, track tool calls and trigger playbook recording when a multi-step task is detected:

```go
func (al *AgentLoop) runLLMIteration(...) (string, int, error) {
    iteration := 0
    toolCallCount := 0  // NEW: track total tool calls

    for iteration < agent.MaxIterations {
        // ... existing LLM call logic ...

        // Count tool calls
        toolCallCount += len(normalizedToolCalls)

        // ... existing tool execution logic ...
    }

    // NEW: After loop ends, check if this was a multi-step task
    if al.pbManager != nil && toolCallCount >= al.cfg.Playbook.DetectionThreshold {
        // Search for existing playbook matching the user's message
        results, _ := al.pbManager.Search(ctx, playbookd.SearchQuery{
            Text:  opts.UserMessage,
            Limit: 1,
        })
        if len(results) > 0 && results[0].Score > 0.5 {
            // Record execution against existing playbook
            al.pbManager.RecordExecution(ctx, &playbookd.ExecutionRecord{
                PlaybookID:  results[0].Playbook.ID,
                PlaybookVer: results[0].Playbook.Version,
                Outcome:     playbookd.OutcomeSuccess,
                TaskContext: opts.UserMessage,
            })
        } else {
            // Inject system message asking agent to create a playbook
            messages = append(messages, providers.Message{
                Role:    "system",
                Content: "You just completed a multi-step task. Summarize it as a playbook using the playbook_record tool.",
            })
            // Run one more LLM iteration to capture the playbook
        }
    }

    return finalContent, iteration, nil
}
```

### Full flow

```
User: "Deploy the staging environment"
  │
  ├─ BuildSystemPrompt includes "Procedural Memory" section
  │   with active playbooks listed
  │
  ├─ Agent sees it has a playbook for "deploy staging"
  │   → calls playbook_search("deploy staging")
  │   → follows the returned steps
  │
  ├─ runLLMIteration tracks tool_call_count
  │
  └─ After loop: tool_call_count >= 3
      → Records execution against existing playbook
      → Playbook confidence increases
      → Next time: agent is better at this task
```

## License

MIT
