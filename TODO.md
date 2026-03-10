# Armage Project Roadmap & TODO

This file tracks the development of **Armage**, a stateless, resilient ReAct coding agent optimized for the Android (Termux) environment.

## 🚀 Current Status (Phase 2)
- [x] **Core ReAct Brain**: Thought -> Action -> Observation loop implemented in Go.
- [x] **State Persistence**: Stateless architecture with JSON-backed history (`.armage_state.json`).
- [x] **OpenRouter Integration**: Multi-model support with `google/gemma-3-12b-it` as default.
- [x] **Surgical Tools**:
    - `shell`: Executes Termux-native commands.
    - `read_file`: Token-efficient reading with line numbers and ranges.
    - `write_file`: Atomic file creation (temp-then-move).
    - `grep_search`: Recursive pattern matching.
    - `edit_file_diff`: Surgical search-and-replace (Find/Replace blocks).

---

## 🛠️ Must-Have Tools (The Next "Hands")
To reach "Elite" status, the agent needs the following high-fidelity tools:

1.  **`list_dir` (Project Map)**
    - *Goal*: Replace messy `ls -R` with a structured, token-efficient directory tree.
    - *Constraint*: Support depth limiting (e.g., 2 levels) and ignore `.git` / `node_modules`.
    - *Status*: ✅ **Complete** (supports depth up to 3)

2.  **`get_symbols` (Code CT Scan)**
    - *Goal*: List all functions, classes, and variables in a file.
    - *Constraint*: Use lightweight regex or `ctags` to avoid heavy parsers.

3.  **`apply_patch` (Advanced Edits)**
    - *Goal*: Support Standard Unified Diffs for complex, multi-line refactors.
    - *Constraint*: More robust than Find/Replace for overlapping blocks.

---

## 🛡️ The "Governor" & UI (Phase 3)
- [ ] **Safety Protocol**: Implement a `PendingAction` state in `Agent.Step` to force manual approval (`[Y/n]`) for destructive actions.
- [ ] **Bubble Tea TUI**:
    - [ ] Real-time "Thinking..." spinners.
    - [ ] Markdown rendering for "Thoughts" and code blocks.
    - [ ] Integrated safety prompts.

---

## 📊 Token & Context Management (Phase 4)
- [ ] **Context Trimming**: Implement a sliding window or summarization strategy for long histories.
- [ ] **Context Pinning**: Allow the agent or user to "pin" critical files (e.g., `main.go`) to prevent them from being trimmed.
- [ ] **Token Usage Tracker**: Log turn-by-turn costs to avoid hitting free-tier limits.

---

## 📝 Immediate Next Steps
1.  **Implement `list_dir`**: Create `pkg/agent/list_dir_tool.go` with depth-limiting.
2.  **Refine `Agent.Step`**: Add the `RequireApproval` flag and a `Status` return to handle safety interrupts.
3.  **Bootstrap TUI**: Move from the simple CLI in `main.go` to the Bubble Tea framework.

---

## 📜 Architectural Mandates (from `req.md`)
*   **Statelessness**: Read context from disk every iteration.
*   **Bionic Compatibility**: Only use Termux-native binaries or pure Go (no Glibc dependencies).
*   **Atomic Edits**: Never write directly to a file; always use `.tmp` then `mv`.
*   **Relative Paths**: Only operate within the project sandbox.
