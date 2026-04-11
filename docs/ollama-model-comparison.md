# Ollama Model Comparison

Head-to-head comparison of local Ollama models for Gavel code analysis, tested 2026-04-11.

## Test Setup

- **Hardware:** Apple Silicon (local inference)
- **Persona:** `code-reviewer` (default, ~50 word prompt)
- **Policy:** `shall-be-merged` (error severity)
- **Test files:**
  - `internal/input/handler.go` — 108 lines, simple I/O (file reading, diff parsing, directory walking)
  - `internal/analyzer/analyzer.go` — 153 lines, complex orchestration (BAML client, caching, SARIF assembly)

## Results

### handler.go (simple file)

| Metric | gemma4:e4b | gemma4:26b | gpt-oss:20b | qwen2.5-coder:7b |
|--------|-----------|------------|-------------|-------------------|
| **Parameters** | ~7B | ~26B | 20B | 7.6B |
| **Time** | 40s | 130s | 69s | 25s |
| **Tokens in/out** | 1137/1677 | 1137/4688 | 1072/3395 | 1010/437 |
| **LLM findings** | 1 | 1 | 1 | 3 |
| **Uses correct ruleId** | No | Yes | Yes | No |
| **Top finding** | Brittle diff parsing (Fields splitting) | Spaces-in-paths bug (Fields splitting) | Symlink traversal via filepath.Walk | Append-in-loop style nit |

**Notes:**
- gemma4:26b and gemma4:e4b both flagged the same code (diff path extraction via `strings.Fields`) but 26b identified the specific edge case (filenames with spaces) while e4b gave a vaguer "brittle parsing" description.
- gpt-oss:20b found a different, valid security concern (symlink following in `filepath.Walk`).
- qwen2.5-coder:7b produced 3 findings but the top one was a style nit (append in loop), not a real issue.

### analyzer.go (complex file)

| Metric | gemma4:e4b | gemma4:26b | gpt-oss:20b | qwen2.5-coder:7b |
|--------|-----------|------------|-------------|-------------------|
| **Parameters** | ~7B | ~26B | 20B | 7.6B |
| **Time** | 42s | 166s | 30s | 6s |
| **Tokens in/out** | 1967/1833 | 1967/6277 | 1784/1669 | 1751/145 |
| **LLM findings** | 2 | 2 | 2 | 1 |
| **Uses correct ruleId** | No | Yes | Yes | No |
| **Findings** | Ignored BuildIndex error; cache design concern | Line-offset bug from prepended header; thread safety | Thread safety on cache; cache key ignores content | Nil check on idx (false positive — already guarded) |

**Notes:**
- gemma4:26b found a unique line-offset bug: prepending `// File: %s\n` shifts all LLM-reported line numbers by +1 relative to the original content. No other model caught this.
- gpt-oss:20b and gemma4:26b both flagged the thread-safety concern on cached fields.
- qwen2.5-coder:7b produced a false positive — it flagged a nil pointer risk on `idx` that is already guarded by `if idx != nil` on line 108.

## Summary

| Rank | Model | Params | Quality | Speed | Instruction Following | Best For |
|------|-------|--------|---------|-------|-----------------------|----------|
| 1 | **gemma4:26b** | ~26B | Excellent | Slow (130-166s) | Strong (correct ruleId) | Thorough local review when time permits |
| 2 | **gpt-oss:20b** | 20B | Strong | Medium (30-69s) | Strong (correct ruleId) | Best speed/quality ratio for local use |
| 3 | **gemma4:e4b** | ~7B | Decent | Medium (40s) | Weak (invents ruleIds) | Quick local checks with acceptable quality |
| 4 | **qwen2.5-coder:7b** | 7.6B | Weak | Fast (6-25s) | Weak (empty/wrong ruleIds) | Fast iteration only; high false-positive rate |

### Key Observations

1. **Instruction following scales with parameters.** Only the 20B+ models (gpt-oss:20b, gemma4:26b) consistently used the actual policy name (`shall-be-merged`) as the `ruleId`. The ~7B models invented their own categories or left it empty, which breaks SARIF rule correlation.

2. **gemma4:26b found a real bug** (line-number offset from prepended filename header) that the other three models missed entirely. Larger models surface subtler issues.

3. **qwen2.5-coder:7b is fastest but least reliable.** It produced a false positive on analyzer.go and noisy style nits on handler.go. Speed doesn't compensate for findings you have to manually filter.

4. **gpt-oss:20b is the best all-rounder for local inference.** Correct ruleIds, actionable findings, and reasonable speed. It's 2-5x faster than gemma4:26b with only a modest quality drop.

## Recommendations

- **Default local model:** `gpt-oss:20b` — best balance of speed, quality, and instruction compliance.
- **Deep local review:** `gemma4:26b` — when you have time and want the most thorough analysis.
- **Quick smoke test:** `gemma4:e4b` — reasonable quality at moderate speed, but ruleId non-compliance means findings won't map to policies in SARIF output.
- **Avoid for gating:** `qwen2.5-coder:7b` — too many false positives and poor instruction following for automated merge/reject decisions.
