package main

import (
	"fmt"
	"os"
)

// main function to parse arguments and initiate the chat request.
func main() {
	args, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := ask(args.Prompt, args.Model); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
