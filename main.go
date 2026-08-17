// Command task046-casstore runs the content-addressable block store.
//
// Use --smoke-test to run the built-in self-check, which exits the process on
// completion.
package main

import (
	"flag"
	"fmt"
	"os"

	"task046-casstore/internal/selfcheck"
)

func main() {
	smoke := flag.Bool("smoke-test", false, "run the built-in self-check and exit")
	flag.Parse()

	if *smoke {
		if err := selfcheck.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "smoke-test FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("smoke-test PASSED")
		return
	}

	fmt.Println("content-addressable block store; use --smoke-test to self-verify.")
}
