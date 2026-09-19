package commands

import (
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func refuseLegacyContextSoT(action string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	return ctxgit.RefuseLegacySoT(wd, action)
}
