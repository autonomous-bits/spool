package remote

import (
	"errors"
	"testing"
)

type fakeKeychain struct {
	value string
	err   error
}

func (f fakeKeychain) Lookup(service, account string) (string, error) {
	return f.value, f.err
}

func TestResolveCredentialPrefersKeychainOverEnvAndPrompt(t *testing.T) {
	opts := ResolveOptions{
		Keychain: fakeKeychain{value: "from-keychain"},
		Getenv:   func(string) string { return "from-env" },
		Prompt:   func(string) (string, error) { return "from-prompt", nil },
	}
	credential, err := ResolveCredential("acme-prod", AuthModeBearer, opts)
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if credential.Value != "from-keychain" || credential.Source != CredentialSourceKeychain {
		t.Fatalf("credential = %#v, want keychain value", credential)
	}
}

func TestResolveCredentialFallsBackToEnvWhenKeychainEmpty(t *testing.T) {
	opts := ResolveOptions{
		Keychain: fakeKeychain{err: ErrCredentialNotFound},
		Getenv: func(name string) string {
			if name == "SPOOL_RACK_TOKEN" {
				return "from-env"
			}
			return ""
		},
		Prompt: func(string) (string, error) { return "from-prompt", nil },
	}
	credential, err := ResolveCredential("acme-prod", AuthModeBearer, opts)
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if credential.Value != "from-env" || credential.Source != CredentialSourceEnv {
		t.Fatalf("credential = %#v, want env value", credential)
	}
}

func TestResolveCredentialFallsBackToPromptWhenKeychainAndEnvEmpty(t *testing.T) {
	opts := ResolveOptions{
		Keychain: fakeKeychain{err: ErrCredentialNotFound},
		Getenv:   func(string) string { return "" },
		Prompt:   func(string) (string, error) { return "from-prompt", nil },
	}
	credential, err := ResolveCredential("acme-prod", AuthModeAPIKey, opts)
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if credential.Value != "from-prompt" || credential.Source != CredentialSourceInteractive {
		t.Fatalf("credential = %#v, want prompt value", credential)
	}
}

func TestResolveCredentialReportsNotFoundWhenNoSourceYieldsValue(t *testing.T) {
	opts := ResolveOptions{
		Keychain: fakeKeychain{err: ErrCredentialNotFound},
		Getenv:   func(string) string { return "" },
		Prompt:   func(string) (string, error) { return "", ErrCredentialNotFound },
	}
	if _, err := ResolveCredential("acme-prod", AuthModeBearer, opts); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("ResolveCredential error = %v, want ErrCredentialNotFound", err)
	}
}

func TestResolveCredentialWithNoSourcesConfiguredReportsNotFound(t *testing.T) {
	if _, err := ResolveCredential("acme-prod", AuthModeBearer, ResolveOptions{}); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("ResolveCredential error = %v, want ErrCredentialNotFound", err)
	}
}

func TestResolveCredentialDefaultsGetenvToOSGetenvWhenNil(t *testing.T) {
	t.Setenv("SPOOL_RACK_TOKEN", "from-real-os-env")
	opts := ResolveOptions{Keychain: fakeKeychain{err: ErrCredentialNotFound}}
	credential, err := ResolveCredential("acme-prod", AuthModeBearer, opts)
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if credential.Value != "from-real-os-env" || credential.Source != CredentialSourceEnv {
		t.Fatalf("credential = %#v, want os.Getenv value", credential)
	}
}
