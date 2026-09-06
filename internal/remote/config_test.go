package remote

import (
	"errors"
	"testing"
)

func validConfig() Config {
	return Config{Endpoint: "https://rack.example.com", RepoID: "acme-prod", AuthMode: AuthModeBearer}
}

func TestConfigValidateAcceptsWellFormedConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestConfigValidateRejectsEmptyEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = ""
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsNonHTTPScheme(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = "ftp://rack.example.com"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsMissingHost(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = "https://"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsEmptyRepoID(t *testing.T) {
	cfg := validConfig()
	cfg.RepoID = ""
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsInvalidAuthMode(t *testing.T) {
	cfg := validConfig()
	cfg.AuthMode = "oauth"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsEndpointWithEmbeddedCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = "https://user:hunter2@rack.example.com"
	if err := cfg.Validate(); !errors.Is(err, ErrSecretLikeValue) {
		t.Fatalf("Validate = %v, want ErrSecretLikeValue", err)
	}
}

func TestConfigValidateRejectsSecretLikeRepoID(t *testing.T) {
	secretLookingValues := []string{
		"ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"sk-abcdefghijklmnopqrstuvwxyz0123456789",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"AKIAIOSFODNN7EXAMPLE",
	}
	for _, value := range secretLookingValues {
		cfg := validConfig()
		cfg.RepoID = value
		if err := cfg.Validate(); !errors.Is(err, ErrSecretLikeValue) {
			t.Fatalf("Validate(%q) = %v, want ErrSecretLikeValue", value, err)
		}
	}
}

func TestConfigValidateRejectsOverlongRepoID(t *testing.T) {
	cfg := validConfig()
	long := make([]byte, maxRepoIDLength+1)
	for i := range long {
		long[i] = 'a'
	}
	cfg.RepoID = string(long)
	if err := cfg.Validate(); !errors.Is(err, ErrSecretLikeValue) {
		t.Fatalf("Validate = %v, want ErrSecretLikeValue", err)
	}
}

func TestConfigValidateRejectsWhitespaceInRepoID(t *testing.T) {
	cfg := validConfig()
	cfg.RepoID = "acme prod"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsLeadingOrTrailingWhitespaceInEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = " https://rack.example.com"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsLeadingOrTrailingWhitespaceInRepoID(t *testing.T) {
	cfg := validConfig()
	cfg.RepoID = " acme-prod"
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidateRejectsEndpointWithQueryOrFragment(t *testing.T) {
	for _, endpoint := range []string{
		"https://rack.example.com/?token=super-secret",
		"https://rack.example.com/#fragment",
	} {
		cfg := validConfig()
		cfg.Endpoint = endpoint
		if err := cfg.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Validate(%q) = %v, want ErrInvalidConfig", endpoint, err)
		}
	}
}

func TestConfigValidateAcceptsRepoIDsThatMerelyContainSecretPrefixSubstrings(t *testing.T) {
	// These must not be flagged: the secret-prefix heuristic matches
	// credential-shaped tokens at a boundary, not any substring occurrence,
	// so ordinary identifiers that happen to contain a prefix are accepted.
	for _, repoID := range []string{"akiametrics", "eyjafjallajokull"} {
		cfg := validConfig()
		cfg.RepoID = repoID
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%q) = %v, want nil", repoID, err)
		}
	}
}

func TestEnvVarMapsAuthModes(t *testing.T) {
	bearer, err := EnvVar(AuthModeBearer)
	if err != nil || bearer != "SPOOL_RACK_TOKEN" {
		t.Fatalf("EnvVar(bearer) = %q, %v", bearer, err)
	}
	apiKey, err := EnvVar(AuthModeAPIKey)
	if err != nil || apiKey != "SPOOL_RACK_API_KEY" {
		t.Fatalf("EnvVar(api_key) = %q, %v", apiKey, err)
	}
	if _, err := EnvVar("oauth"); err == nil {
		t.Fatal("EnvVar(oauth) = nil error, want error")
	}
}
