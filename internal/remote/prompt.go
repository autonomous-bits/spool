package remote

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// TerminalPrompt returns a PromptFunc that reads hidden input from stdin
// when it is an interactive terminal, echoing the label to stdout first. It
// returns ErrCredentialNotFound (wrapped) when stdin is not a terminal,
// rather than blocking on a pipe or redirected input.
func TerminalPrompt() PromptFunc {
	return func(label string) (string, error) {
		return terminalPrompt(os.Stdin, os.Stdout, label)
	}
}

func terminalPrompt(stdin *os.File, stdout io.Writer, label string) (string, error) {
	if !isatty.IsTerminal(stdin.Fd()) && !isatty.IsCygwinTerminal(stdin.Fd()) {
		return "", ErrCredentialNotFound
	}
	if _, err := fmt.Fprint(stdout, label); err != nil {
		return "", err
	}
	value, err := term.ReadPassword(int(stdin.Fd()))
	if _, newlineErr := fmt.Fprintln(stdout); newlineErr != nil && err == nil {
		err = newlineErr
	}
	if err != nil {
		return "", fmt.Errorf("read hidden input: %w", err)
	}
	return string(value), nil
}
