# Plan for sys_info tool

## Goal
Create a new tool called `sys_info` that returns the operating system and Go version.

## Steps
1. Create a new directory for the tool: `cmd/sys_info/`
2. Write a Go main function in `cmd/sys_info/main.go` that:
   - Uses `runtime.GOOS`, `runtime.GOARCH`, `runtime.Version()` to get system info
   - Outputs the information in a clear format (e.g., JSON or key-value pairs)
3. Ensure the tool is buildable by adding it to the Go module if necessary (likely already covered by `./cmd/...`)
4. Test the tool by running `go run ./cmd/sys_info`
5. Verify the output is correct
6. Update documentation if needed (e.g., README)

## Criteria for Completion
- Tool compiles without errors
- When executed, prints OS and Go version information
- No existing functionality broken

## Progress
- [ ] Create sys_info tool