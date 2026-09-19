package ctxgit

import (
	"errors"
	"fmt"
)

// LegacyContextSoTError reports that a Rack or `.spl` path is not the durable
// solution-context source of truth. Callers should bind with BindRelPath and
// sync via stock git clone, pull requests, and history.
func LegacyContextSoTError(action string) error {
	if action == "" {
		action = "this command"
	}
	return fmt.Errorf("%s is not the durable solution-context source of truth: bind each code repo with %s (or `spl context init --remote <url>`) and sync with stock git clone/PR/history; `.spl` and Rack remotes are out of the happy path", action, BindRelPath)
}

// BoundFrom reports whether start resolves an explicit context bind.
func BoundFrom(start string) (Bind, bool, error) {
	_, _, bind, err := FindBind(start)
	if err == nil {
		return bind, true, nil
	}
	if errors.Is(err, ErrUnbound) {
		return Bind{}, false, nil
	}
	return Bind{}, false, err
}

// RefuseLegacySoT returns LegacyContextSoTError when start is bound to a
// solution context remote. Unbound directories are left unchanged so local
// graph-VCS tests and tooling keep working until a bind exists.
func RefuseLegacySoT(start, action string) error {
	_, bound, err := BoundFrom(start)
	if err != nil {
		return err
	}
	if bound {
		return LegacyContextSoTError(action)
	}
	return nil
}
