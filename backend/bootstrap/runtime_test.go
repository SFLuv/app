package bootstrap

import "testing"

func TestResolveDBPoolNamesUsesDefaults(t *testing.T) {
	t.Setenv(botDBNameEnvKey, "")
	t.Setenv(appDBNameEnvKey, "")
	t.Setenv(ponderDBNameEnvKey, "")

	botDBName, appDBName := resolveDBPoolNames()
	ponderDBName := resolvePonderDBPoolName()

	if botDBName != defaultBotDBName {
		t.Fatalf("expected default bot db name %q, got %q", defaultBotDBName, botDBName)
	}
	if appDBName != defaultAppDBName {
		t.Fatalf("expected default app db name %q, got %q", defaultAppDBName, appDBName)
	}
	if ponderDBName != defaultPonderDBName {
		t.Fatalf("expected default ponder db name %q, got %q", defaultPonderDBName, ponderDBName)
	}
}

func TestResolveDBPoolNamesUsesEnvOverrides(t *testing.T) {
	t.Setenv(botDBNameEnvKey, "custom-bot")
	t.Setenv(appDBNameEnvKey, "custom-app")
	t.Setenv(ponderDBNameEnvKey, "custom-ponder")

	botDBName, appDBName := resolveDBPoolNames()
	ponderDBName := resolvePonderDBPoolName()

	if botDBName != "custom-bot" {
		t.Fatalf("expected bot db name override, got %q", botDBName)
	}
	if appDBName != "custom-app" {
		t.Fatalf("expected app db name override, got %q", appDBName)
	}
	if ponderDBName != "custom-ponder" {
		t.Fatalf("expected ponder db name override, got %q", ponderDBName)
	}
}
