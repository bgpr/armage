# Armage Project Roadmap & TODO

This file tracks the development of **Armage**, a stateless, resilient ReAct coding agent optimized for the Android (Termux) environment.

## 🚀 Current Status (Phase 3 Complete)
- [x] **Core ReAct Brain**: Thought -> Action -> Observation loop implemented in Go.
- [x] **State Persistence**: Stateless architecture with JSON-backed history (`.armage_state.json`).
- [x] **Advanced Polyglot Parser**: Balanced JSON/ReAct parsing with aggressive cleanup.
- [x] **Surgical Tools**: `shell`, `read_file`, `write_file`, `grep_search`, `list_dir`, `get_symbols`, `apply_patch`.
- [x] **Pro Dashboard (TUI)**:
    - [x] Focus Mode (F3) and Plan View (F4).
    - [x] Markdown rendering with caching.
    - [x] Modal Focus (Tab) for reliable scrolling/typing.
    - [x] Input history cycling (Ctrl+P/N).
    - [x] Live Mission Progress tracker.

---

## 🛠️ Performance & Intelligence (Phase 4)
- [x] **Privacy Shield**: Local PII scrubbing via `llama-server`.
- [x] **Context Pinning**: Prevent critical files from being trimmed.
- [x] **Multi-Provider Resilience**: Support for OpenRouter with automated 429/403/402 rotation.
- [ ] **Context Refresh 2.0**: 
    - [ ] Auto-Summarization of long histories to prevent "drift".
    - [ ] Sliding window token management.

---

## 📝 Immediate Next Steps (Fundamental Fixes)
1.  **Agent Reasoning Resilience**: 
    - [ ] Implement **Listing Loop Detection** (nudge agent if it repeats `list_dir`).
    - [ ] Harden System Prompt to force action over exploration.
2.  **UX Polish**:
    - [ ] **Touch/Tap focus switching**: Enable switching between input and viewport via screen taps.
    - [ ] **Auto-Focus input**: Ensure input is focused after tool approval.
3.  **Optimization**:
    - [ ] MRU model cache for faster rotations.
    - [ ] Bloom filter for known-busy models.

---

## 📜 Architectural Mandates (from `req.md`)
*   **Statelessness**: Read context from disk every iteration.
*   **Bionic Compatibility**: Only use Termux-native binaries or pure Go.
*   **Atomic Edits**: Never write directly to a file; always use `.tmp` then `mv`.
*   **Relative Paths**: Only operate within the project sandbox.
