package app_test

import (
	"testing"

	"codeplat/config"
	"codeplat/internal/app"
	"codeplat/internal/apptest"
)

func TestNewWithAuthAdmitsTestToken(t *testing.T) {
	p, err := apptest.NewWithAuth(t).Auth.Authenticate("alice-token")
	if err != nil {
		t.Fatalf("Authenticate(alice-token): %v", err)
	}
	if p.Name != "alice" {
		t.Errorf("Authenticate(alice-token).Name = %q, want %q", p.Name, "alice")
	}
}

func TestNewWithAuthRefusesEmptyToken(t *testing.T) {
	if _, err := apptest.NewWithAuth(t).Auth.Authenticate(""); err == nil {
		t.Error("Authenticate(\"\"): want error for closed app, got nil")
	}
}

func TestNewIsOpen(t *testing.T) {
	if _, err := apptest.New(t).Auth.Authenticate(""); err != nil {
		t.Errorf("Authenticate(\"\"): want no error for open app, got %v", err)
	}
}

func TestNewRejectsClosedConfigWithEmptyToken(t *testing.T) {
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Auth.Open = false
	cfg.Auth.Tokens = map[string]string{"a": ""}
	a, err := app.New(cfg)
	if err == nil {
		t.Fatal("app.New: want error for empty token, got nil")
	}
	if a != nil {
		t.Errorf("app.New: app = %v, want nil", a)
	}
}

func TestNewRunnerRequiresCommand(t *testing.T) {
	// The test config has no runner section, so there is no command to run.
	r, err := apptest.New(t).NewRunner()
	if err == nil {
		t.Fatal("NewRunner: want error for missing runner.command")
	}
	if r != nil {
		t.Errorf("NewRunner: runner = %v, want nil", r)
	}
}
