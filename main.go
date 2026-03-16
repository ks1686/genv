package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO(M1): wire up CLI commands.
	fmt.Fprintf(os.Stderr, "gpm: no command specified\n")
	os.Exit(1)
}
