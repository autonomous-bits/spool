// Command spl is the entry point for the spl CLI.
package main

import (
	"os"
)

func main() {
	command, closeFn := bootstrapRootCommand(os.Stdout)
	command.SetArgs(os.Args[1:])
	if err := command.Execute(); err != nil {
		newLogger(os.Stderr).Error("command failed", "error", err)
		_ = closeFn()
		os.Exit(1)
	}
	if err := closeFn(); err != nil {
		newLogger(os.Stderr).Error("close", "error", err)
		os.Exit(1)
	}
}
