package bootstrap

import (
	"context"
	"fmt"
)

// migrateSupervisorWorkflowCapability is the 1.71 migration body: the two
// supervisor workflow capabilities become one.
//
// "View assigned workflows" and "Export workflow data" were separate grants
// over the same data. The export is the same rows the view already shows, in a
// file — so the split offered a distinction that does not exist: anybody who
// can read the list can read the list, and withholding the download while
// allowing the page protects nothing.
//
// Merged onto the view key, which is the one that stays. Anybody who held only
// the export key is granted the survivor first, so the merge cannot take
// access away from somebody who had it — then the retired rows go. Both steps
// run in one statement batch, so a failure between them cannot leave somebody
// holding neither.
func migrateSupervisorWorkflowCapability(ctx context.Context, pools *MigrationPools) error {
	if _, err := pools.App.Exec(ctx, `
		-- Granted before the delete, and ON CONFLICT DO NOTHING because most
		-- holders of export already hold view. role_type and granted_by are
		-- carried across so the row still says where it came from.
		INSERT INTO organization_member_permissions
			(organization_id, user_id, role_type, capability, granted_by, created_at)
		SELECT organization_id, user_id, role_type, 'supervisor.workflows.view', granted_by, created_at
		FROM organization_member_permissions
		WHERE capability = 'supervisor.workflows.export'
		ON CONFLICT (organization_id, user_id, capability) DO NOTHING;

		DELETE FROM organization_member_permissions
		WHERE capability = 'supervisor.workflows.export';
	`); err != nil {
		return fmt.Errorf("error merging the supervisor workflow capabilities: %w", err)
	}

	return nil
}
