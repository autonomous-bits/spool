package ctxgit

import (
	"errors"
	"fmt"
)

// LegacyContextSoTError reports that a Rack or `.spl` path is deprecated and
// unsupported for solution context. Callers should bind with BindRelPath and
// sync via stock git clone, pull requests, and history. Leftover `.spl` is
// migration-only (`spl context export`), not a parallel SoT.
func LegacyContextSoTError(action string) error {
	if action == "" {
		action = "this command"
	}
	return fmt.Errorf("%s is deprecated and unsupported for solution context: bind each code repo with %s (or `spl context init --remote <url>`) and sync with stock git clone/PR/history; bind + stock git is the only durable SoT; leftover `.spl`/Rack is migration-only (`spl context export`), not a parallel happy path", action, BindRelPath)
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
// solution context remote. Unbound leftover `.spl` is left unchanged so
// migrate-once export can still read it; it is not a supported parallel SoT.
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
