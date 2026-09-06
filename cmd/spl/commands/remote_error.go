package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/spf13/cobra"
)

// remoteErrorEnvelope is the consistent JSON shape every remote-originated
// command failure (unauthorized, malformed request, conflict, internal
// error, or an unreachable remote) is reported with on the command's
// output. It mirrors Rack's own JSON error envelope so an operator can
// always find a correlationId to match against Rack's audit log, regardless
// of which command or failure kind produced it.
type remoteErrorEnvelope struct {
	Error         string `json:"error"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlationId,omitempty"`
	CurrentHead   string `json:"currentHead,omitempty"`
}

// remoteErrorCode returns a stable, machine-readable fallback code for err
// when Rack itself did not supply one (e.g. the remote was unreachable, so
// no JSON body was ever decoded).
func remoteErrorCode(err error) string {
	switch {
	case errors.Is(err, remote.ErrRemoteUnreachable):
		return "remote_unreachable"
	case errors.Is(err, remote.ErrPushRejected):
		return "push_rejected"
	case errors.Is(err, remote.ErrPullRejected):
		return "pull_rejected"
	case errors.Is(err, remote.ErrPullBranchNotFound):
		return "pull_branch_not_found"
	case errors.Is(err, remote.ErrBranchAlreadyExists):
		return "branch_already_exists"
	case errors.Is(err, remote.ErrBranchSourceNotFound):
		return "branch_source_not_found"
	case errors.Is(err, remote.ErrBranchNotFound):
		return "branch_not_found"
	case errors.Is(err, remote.ErrBranchRejected):
		return "branch_rejected"
	default:
		var protectedErr *remote.ProtectedBranchError
		if errors.As(err, &protectedErr) {
			return "branch_protected"
		}
		return "remote_error"
	}
}

// newRemoteErrorEnvelope builds the consistent JSON envelope for a
// remote-originated failure err. credential, when non-empty, is redacted
// from the envelope's message as defense in depth, even though the remote
// package itself already redacts credentials from error text.
func newRemoteErrorEnvelope(err error, credential string) remoteErrorEnvelope {
	envelope := remoteErrorEnvelope{Error: remoteErrorCode(err), Message: err.Error()}
	var rackErr *remote.RackError
	if errors.As(err, &rackErr) {
		if rackErr.Code != "" {
			envelope.Error = rackErr.Code
		}
		if rackErr.Message != "" {
			envelope.Message = rackErr.Message
		}
		envelope.CorrelationID = rackErr.CorrelationID
		envelope.CurrentHead = rackErr.CurrentHead
	}
	var protectedErr *remote.ProtectedBranchError
	if errors.As(err, &protectedErr) {
		envelope.Message = protectedErr.Error()
		envelope.CorrelationID = protectedErr.CorrelationID
	}
	envelope.Message = remote.Redact(envelope.Message, credential)
	return envelope
}

// writeRemoteErrorEnvelope writes the consistent JSON error envelope for a
// remote-originated failure err to command's output, then returns an error
// wrapping err (annotated with verb) so the command still reports a
// non-zero exit status. If the envelope itself fails to encode, that
// encoding error is returned instead.
func writeRemoteErrorEnvelope(command *cobra.Command, verb string, err error, credential string) error {
	envelope := newRemoteErrorEnvelope(err, credential)
	if encodeErr := json.NewEncoder(command.OutOrStdout()).Encode(envelope); encodeErr != nil {
		return encodeErr
	}
	return fmt.Errorf("%s: %w", verb, err)
}
