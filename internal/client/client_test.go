package client

import (
	"context"
	"os"
	"testing"

	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
)

// testToken is a valid Admiral PAT format token for use in tests.
const testToken = "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG"

func TestCreateClient_NotLoggedIn(t *testing.T) {
	os.Unsetenv(credentials.EnvToken)

	opts := &Options{
		ServerAddr: "localhost:9999",
		ConfigDir:  t.TempDir(), // empty dir, no credentials
	}

	_, err := CreateClient(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when not logged in")
	}
	if want := "no token configured"; !contains(err.Error(), want) {
		t.Fatalf("error should contain %q, got %q", want, err.Error())
	}
}

func TestCreateClient_WithEnvToken(t *testing.T) {
	t.Setenv(credentials.EnvToken, testToken)

	opts := &Options{
		ServerAddr: "localhost:9999",
		PlainText:  true,
		ConfigDir:  t.TempDir(),
	}

	// With a valid token from env and plaintext mode, CreateClient should
	// succeed (the SDK dials lazily with gRPC).
	c, err := CreateClient(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer c.Close()
}

func TestCreateClient_InsecureFlag(t *testing.T) {
	t.Setenv(credentials.EnvToken, testToken)

	opts := &Options{
		ServerAddr: "localhost:9999",
		Insecure:   true,
		ConfigDir:  t.TempDir(),
	}

	c, err := CreateClient(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer c.Close()
}

func TestCreateClient_WithVerbose(t *testing.T) {
	t.Setenv(credentials.EnvToken, testToken)

	opts := &Options{
		ServerAddr: "localhost:9999",
		PlainText:  true,
		Verbose:    true,
		ConfigDir:  t.TempDir(),
	}

	c, err := CreateClient(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer c.Close()
}

func TestOptions_Defaults(t *testing.T) {
	opts := &Options{}

	if opts.ServerAddr != "" {
		t.Fatalf("expected empty ServerAddr, got %q", opts.ServerAddr)
	}
	if opts.Insecure {
		t.Fatal("expected Insecure to be false")
	}
	if opts.PlainText {
		t.Fatal("expected PlainText to be false")
	}
	if opts.Verbose {
		t.Fatal("expected Verbose to be false")
	}
	if opts.OutputFormat != "" {
		t.Fatalf("expected empty OutputFormat, got %q", opts.OutputFormat)
	}
}

func TestOptions_OutputFormat(t *testing.T) {
	opts := &Options{
		OutputFormat: output.FormatJSON,
	}

	if opts.OutputFormat != output.FormatJSON {
		t.Fatalf("expected %q, got %q", output.FormatJSON, opts.OutputFormat)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
