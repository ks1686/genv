//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"github.com/ks1686/genv/internal/genvfile"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test_svc_read <file>")
		os.Exit(1)
	}
	f, err := genvfile.Read(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("Schema: %s\n", f.SchemaVersion)
	fmt.Printf("Services: %+v\n", f.Services)
	for name, svc := range f.Services {
		fmt.Printf("  %s: %+v\n", name, svc)
	}
}
