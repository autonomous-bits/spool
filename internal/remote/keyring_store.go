package remote

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// KeyringStore resolves credentials from the operating system's native
// keychain or secret store via github.com/zalando/go-keyring.
type KeyringStore struct{}

// Lookup returns the secret stored for service/account, or
// ErrCredentialNotFound if no entry exists or the platform has no available
// secret store. It never returns platform-specific error types to callers.
func (KeyringStore) Lookup(service, account string) (string, error) {
	value, err := keyring.Get(service, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) || errors.Is(err, keyring.ErrUnsupportedPlatform) {
			return "", ErrCredentialNotFound
		}
		// Any other keychain failure (locked store, denied access, etc.) is
		// treated as "not found" so resolution can degrade gracefully to the
		// next credential source rather than failing outright.
		return "", ErrCredentialNotFound
	}
	return value, nil
}
