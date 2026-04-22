package db

import "testing"

func TestMakeDbConnStringPrefersDBBaseURL(t *testing.T) {
	t.Setenv("DB_BASE_URL", "base-host:5432")
	t.Setenv("DB_URL", "legacy-host:5432")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "secret")

	connString := MakeDbConnString("app")

	expected := "postgres://appuser:secret@base-host:5432/app"
	if connString != expected {
		t.Fatalf("expected %q, got %q", expected, connString)
	}
}

func TestMakeDbConnStringFallsBackToLegacyDBURL(t *testing.T) {
	t.Setenv("DB_BASE_URL", "")
	t.Setenv("DB_URL", "legacy-host:5432")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "secret")

	connString := MakeDbConnString("app")

	expected := "postgres://appuser:secret@legacy-host:5432/app"
	if connString != expected {
		t.Fatalf("expected %q, got %q", expected, connString)
	}
}

func TestMakeDbConnStringFallsBackToDefaults(t *testing.T) {
	t.Setenv("DB_BASE_URL", "")
	t.Setenv("DB_URL", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")

	connString := MakeDbConnString("app")

	expected := "postgres://postgres:@localhost:5432/app"
	if connString != expected {
		t.Fatalf("expected %q, got %q", expected, connString)
	}
}
