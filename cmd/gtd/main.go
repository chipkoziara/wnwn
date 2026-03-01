package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("g-tuddy: a GTD TUI app")
	fmt.Println("usage: gtd [command]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  add    Add a task to the inbox")
	os.Exit(0)
}
