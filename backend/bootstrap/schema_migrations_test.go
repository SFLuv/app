package bootstrap

import "testing"

// The registered migration list must be valid: unique versions, strictly
// increasing, each with a description and an apply function.
//
// This is a merge guard. Migrations are appended at the end of one list by
// every branch, so two branches developed in parallel naturally reach for the
// same next version number and git resolves the text without noticing. A
// duplicate is not cosmetic: RunPendingMigrations tracks a single version
// watermark and applies only migrations greater than it, so whichever
// same-numbered migration loses the race is skipped permanently on every
// database that already recorded that version — a silent schema divergence
// that surfaces much later as a missing table or index.
func TestSchemaMigrationSequenceIsValid(t *testing.T) {
	if err := validateMigrationSequence(); err != nil {
		t.Fatalf("schema migration list is invalid: %s", err)
	}
}

// Guards the same property from the other direction, with a message that names
// the offender directly, since validateMigrationSequence stops at the first
// problem it finds.
func TestSchemaMigrationVersionsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, migration := range schemaMigrations {
		if previous, exists := seen[migration.Version]; exists {
			t.Errorf(
				"duplicate migration version %s used by both %q and %q — renumber one; a duplicate is silently skipped on databases already at that version",
				migration.Version, previous, migration.Description,
			)
			continue
		}
		seen[migration.Version] = migration.Description
	}
}

// latestDBVersion drives the "database is newer than this server" check, so it
// has to be the last entry rather than the largest by string comparison.
func TestLatestDBVersionMatchesFinalMigration(t *testing.T) {
	if len(schemaMigrations) == 0 {
		t.Skip("no migrations registered")
	}

	expected := schemaMigrations[len(schemaMigrations)-1].Version
	if got := latestDBVersion(); got != expected {
		t.Fatalf("latestDBVersion() = %s, want %s", got, expected)
	}

	// Every registered version must also compare as newer than the baseline,
	// which catches a malformed version string that would otherwise only fail
	// at boot against a real database.
	for _, migration := range schemaMigrations {
		newer, err := isVersionGreater(migration.Version, baselineDBVersion)
		if err != nil {
			t.Errorf("migration %s has an unparseable version: %s", migration.Version, err)
			continue
		}
		if !newer {
			t.Errorf("migration %s is not greater than the baseline %s", migration.Version, baselineDBVersion)
		}
	}
}
