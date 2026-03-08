# Context: Technical Constraints for AI Coding Agents on Android (Termux)

This document summarizes the technical limitations and architectural requirements for running AI coding agents in a Termux environment on Android.

## 1. Why Desktop-Grade CLI Agents Fail on Android
Existing CLI agents (e.g., Aider, Cline) are optimized for standard Linux distributions and frequently fail on Android due to:

* **ABI/Linker Conflicts:** Android uses the **Bionic C library** rather than the standard **Glibc** found in desktop Linux. Pre-compiled binaries or tools dynamically linked to Glibc will fail to execute ("file not found" errors) even if the file exists.
* **Native Compilation Barriers:** Tools relying on `npm install` or `cargo build` often require native C++/Rust compilation. These processes fail because Termux lacks the comprehensive build environments (`gcc`, `make`, system headers) and the Android NDK required to link against mobile-specific libraries.
* **Sandbox Restrictions:** Android strictly sandboxes application directories. CLI tools often hardcode paths like `/tmp`, `/var`, or `/usr/local`, which are inaccessible. This results in `EACCES` (permission denied) or `ENOENT` (file not found) errors.
* **Architecture Mismatches:** Installers often use "target triple" detection (e.g., `x86_64-unknown-linux-gnu`) that fails to recognize Android's `aarch64-linux-android` architecture, leading to "Arch not supported" errors.

## 2. Requirements for a Mobile-First AI Agent
To build a functional, stable agent on Android, the implementation must adhere to these constraints:

### Stateless, File-Backed Architecture
* **Persistence:** Do not store session state in RAM. Use a **stateless** approach where the agent reads its context (plans, logs, task status) from local files (e.g., `.agent_state.json`) at the start of every iteration.
* **Resilience:** If the Android OS kills the process (battery management), the agent can resume instantly by re-reading the state files from disk.

### Technology Stack
* **Avoid Compiled Dependencies:** Use **pure Python** or standard shell scripts. Do not use tools requiring `node-gyp` or native Rust modules.
* **Native Utilities:** Utilize Termux-provided binaries (`grep`, `sed`, `cat`, `awk`). These are pre-patched by the Termux team to function correctly with the Bionic library.
* **Relative Paths:** Use local paths (e.g., `./.tmp/`) rather than absolute system paths to comply with the Android sandbox.

### Operational Best Practices
* **Plan-Act-Reflect:** Force the agent to write a plan to disk and wait for user confirmation before executing "destructive" shell commands (`rm`, `mv`, `git commit`).
* **Atomic Edits:** Always write changes to a temporary file before moving it to the target location (`mv`) to prevent partial file corruption.
* **Model Strategy:**
    * **Planning/Routine:** Use lightweight models (e.g., **Gemini Flash**) to maintain speed and stay within free-tier rate limits.
    * **Complex Logic:** Reserve reasoning-capable or "thinking" models for debugging steps where the routine model fails.

## 3. Implementation Blueprint
A robust CLI agent for Termux should be structured as four decoupled Python components:
1. **Orchestrator:** A `while` loop managing control flow and API calls.
2. **State Keeper:** Logic to read/write state files to maintain grounding.
3. **Tool Adapter:** Maps LLM "Tool Call" JSON to native shell command execution.
4. **Context Manager:** Trims history/logs to manage token usage and context bloat.
