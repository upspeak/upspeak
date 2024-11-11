package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:]))
}

func Run(args []string) int {
	return 0
}
