package ctxgit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitRunner executes stock git. There is no Spool-specific remote protocol.
// Tests may set Env to isolate git configuration.
type GitRunner struct {
	Bin string
	Env []string
}

func defaultGit() GitRunner {
	return GitRunner{Bin: "git"}
}

func (g GitRunner) bin() string {
	if g.Bin == "" {
		return "git"
	}
	return g.Bin
}

// Run invokes stock git in dir. There is no Spool-specific remote protocol.
func (g GitRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, g.bin(), args...)
	cmd.Dir = dir
	cmd.Env = g.environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (g GitRunner) environ() []string {
	env := os.Environ()
	if len(g.Env) > 0 {
		env = append(env, g.Env...)
	}
	return env
}

func (g GitRunner) HasRev(ctx context.Context, dir, rev string) bool {
	_, err := g.Run(ctx, dir, "rev-parse", "--verify", rev)
	return err == nil
}
