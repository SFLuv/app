package bootstrap

import "testing"

func TestResolveDBPoolNamesUsesDefaults(t *testing.T) {
	t.Setenv(botDBNameEnvKey, "")
	t.Setenv(appDBNameEnvKey, "")

	botDBName, appDBName := resolveDBPoolNames()

	if botDBName != defaultBotDBName {
		t.Fatalf("expected default bot db name %q, got %q", defaultBotDBName, botDBName)
	}
	if appDBName != defaultAppDBName {
		t.Fatalf("expected default app db name %q, got %q", defaultAppDBName, appDBName)
	}
}

func TestResolveDBPoolNamesUsesEnvOverrides(t *testing.T) {
	t.Setenv(botDBNameEnvKey, "custom-bot")
	t.Setenv(appDBNameEnvKey, "custom-app")

	botDBName, appDBName := resolveDBPoolNames()

	if botDBName != "custom-bot" {
		t.Fatalf("expected bot db name override, got %q", botDBName)
	}
	if appDBName != "custom-app" {
		t.Fatalf("expected app db name override, got %q", appDBName)
	}
}
