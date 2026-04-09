// This is a test
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

type SysInfo struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	GoVersion string `json:"go_version"`
}

func main() {
	info := SysInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshalling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonData))
}