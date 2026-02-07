# LSP Setup Guide

This guide explains how to configure gavel as a Language Server in various editors for real-time code analysis.

## Quick Start

1. **Build gavel**:
   ```bash
   go build -o gavel ./cmd/gavel
   ```

2. **Test LSP mode**:
   ```bash
   ./gavel lsp --help
   ```

3. **Configure your editor** (see sections below)

## Editor Configuration

### Neovim (Native LSP)

Add to your `init.lua`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "go", "python", "typescript", "javascript" },
  callback = function()
    vim.lsp.start({
      name = "gavel",
      cmd = { "/path/to/gavel", "lsp" },
      root_dir = vim.fs.dirname(vim.fs.find({ ".gavel", ".git" }, { upward = true })[1]),
    })
  end,
})
```

### Neovim (nvim-lspconfig)

Add to your `init.lua`:

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- Register gavel LSP
if not configs.gavel then
  configs.gavel = {
    default_config = {
      cmd = { '/path/to/gavel', 'lsp' },
      filetypes = { 'go', 'python', 'typescript', 'javascript', 'typescriptreact', 'javascriptreact' },
      root_dir = lspconfig.util.root_pattern('.gavel', '.git'),
    },
  }
end

-- Setup gavel
lspconfig.gavel.setup({})
```

### Helix

Add to `~/.config/helix/languages.toml`:

```toml
[[language]]
name = "go"
language-servers = ["gopls", "gavel"]

[[language]]
name = "python"
language-servers = ["pylsp", "gavel"]

[[language]]
name = "typescript"
language-servers = ["typescript-language-server", "gavel"]

[[language]]
name = "javascript"
language-servers = ["typescript-language-server", "gavel"]

[language-server.gavel]
command = "/path/to/gavel"
args = ["lsp"]
```

### VS Code

Create `.vscode/settings.json` in your project:

```json
{
  "gavel.lspPath": "/path/to/gavel",
  "gavel.enable": true
}
```

Then install the gavel VS Code extension (coming soon) or use a generic LSP client extension.

## Configuration

Gavel LSP uses tiered configuration:

1. **System defaults** - Built into gavel
2. **Machine config** - `~/.config/gavel/policies.yaml`
3. **Project config** - `.gavel/policies.yaml` (highest priority)

### LSP-Specific Options

Add to your `policies.yaml`:

```yaml
lsp:
  watcher:
    debounce_duration: "5m"  # Wait 5 minutes after last edit before analyzing
    watch_patterns:
      - "**/*.go"
      - "**/*.py"
      - "**/*.ts"
      - "**/*.tsx"
      - "**/*.js"
      - "**/*.jsx"
    ignore_patterns:
      - "**/node_modules/**"
      - "**/.git/**"
      - "**/vendor/**"
      - "**/.gavel/**"

  analysis:
    parallel_files: 3     # Analyze up to 3 files in parallel
    priority: "recent"    # Analyze recently-edited files first

  cache:
    ttl: "168h"          # Cache results for 7 days
    max_size_mb: 500     # Maximum cache size
```

### Example: Fast Local Analysis

For rapid feedback with a local model:

```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

lsp:
  watcher:
    debounce_duration: "30s"  # Faster analysis
  analysis:
    parallel_files: 5         # More parallelism
  cache:
    ttl: "24h"                # Shorter cache for rapid iteration

policies:
  shall-be-merged:
    enabled: true
    severity: error
    instruction: "Flag critical issues only - SQL injection, XSS, command injection"
```

## How It Works

1. **File Save** - Editor saves a file
2. **Debounce** - Gavel waits for the configured debounce period (default: 5 minutes)
3. **Cache Check** - Gavel checks if cached results exist for this file + policy combination
4. **Analysis** - If no cache hit, gavel analyzes the file with the configured LLM
5. **Diagnostics** - Results are converted to LSP diagnostics
6. **Publish** - Diagnostics are sent to the editor and displayed inline

### Cache Key

Cache keys are based on:
- File content hash (SHA-256)
- Provider and model name
- Enabled policies and their instructions
- BAML template version

Results are shared across environments when these match (e.g., CI and local).

## Troubleshooting

### No Diagnostics Appear

**Check LSP is running:**
```bash
# Neovim
:LspInfo

# Helix
:lsp-status
```

**Check logs:**
- Gavel LSP logs to stderr
- In Neovim, check `:messages` or `:LspLog`
- In Helix, check the editor logs

**Verify configuration:**
```bash
# Test that config loads
./gavel lsp --help
```

### Analysis is Slow

**Use a faster model:**
```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b  # Fast local model
```

**Reduce debounce:**
```yaml
lsp:
  watcher:
    debounce_duration: "30s"  # Analyze sooner after edits
```

**Increase parallelism:**
```yaml
lsp:
  analysis:
    parallel_files: 5  # Analyze more files simultaneously
```

### Cache Not Working

**Check cache directory exists:**
```bash
ls -la ~/.cache/gavel
```

**Verify cache is enabled:**
```bash
./gavel lsp --cache-dir ~/.cache/gavel
```

**Clear cache if needed:**
```bash
rm -rf ~/.cache/gavel/*
```

**Check cache hits:**
Gavel logs cache hits/misses to stderr when running.

### Diagnostics Persist After Fix

This is normal - diagnostics persist until the file is re-analyzed. Save the file again to trigger a new analysis after the debounce period.

## Advanced Usage

### Custom Cache Location

```bash
./gavel lsp --cache-dir /custom/cache/path
```

### Different Config Files

```bash
./gavel lsp \
  --machine-config ~/.config/gavel/custom.yaml \
  --project-config ./.gavel/custom-policies.yaml
```

### Disable Cache

```bash
./gavel lsp --cache-dir ""
```

## Performance Tips

1. **Use local models for speed** - Ollama with fast models (qwen2.5-coder:7b, deepseek-coder-v2:16b)
2. **Tune debounce** - Longer debounce = less frequent analysis, but stale diagnostics
3. **Enable caching** - Dramatically speeds up repeated analysis
4. **Limit file types** - Only enable for files you want analyzed (via watch_patterns)
5. **Adjust parallel_files** - More parallelism = faster for multi-file edits, but higher resource usage

## Integration with CI

Use the same `.gavel/policies.yaml` for both LSP and CI:

```bash
# In CI
./gavel analyze --dir . --policies .gavel

# In editor (LSP)
# Uses same .gavel/policies.yaml automatically
```

Cache can be shared between CI and local when using the same provider, model, and policies.
