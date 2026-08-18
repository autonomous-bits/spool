// Command spl is the entry point for the spl CLI.
package main

import (
	"os"
)

func main() {
	stateDir, err := repositoryStateDir()
	if err != nil {
		newLogger(os.Stderr).Error("locate repository", "error", err)
		os.Exit(1)
	}
	command, closeRepository := bootstrapRootCommand(os.Stdout, stateDir)
	command.SetArgs(os.Args[1:])
	if err := command.Execute(); err != nil {
		newLogger(os.Stderr).Error("command failed", "error", err)
		if closeErr := closeRepository(); closeErr != nil {
			newLogger(os.Stderr).Error("close repository", "error", closeErr)
		}
		os.Exit(1)
	}
	if err := closeRepository(); err != nil {
		newLogger(os.Stderr).Error("close repository", "error", err)
		os.Exit(1)
	}
}
