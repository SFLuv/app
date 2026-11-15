package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// expireUnclaimedWorkflows releases the faucet reservation held by approved
// one-time workflows that nobody claimed within 30 days of their start, and
// tells each proposer. See db.ExpireUnclaimedOpenEndedWorkflows.
func (s *WorkflowMaintenanceScheduler) expireUnclaimedWorkflows(parent context.Context) {
	app := s.app

	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workflowMaintenanceTimeout)
	defer cancel()

	notices, err := app.db.ExpireUnclaimedOpenEndedWorkflows(ctx)
	if err != nil {
		app.logger.Logf("error expiring unclaimed workflows: %s", err)
		return
	}
	for _, notice := range notices {
		app.logger.Logf(
			"workflow %s (%q) expired: unclaimed %d days after its start; released %d SFLUV",
			notice.WorkflowId, notice.WorkflowTitle, db.UnclaimedWorkflowExpiryDays, notice.ReleasedBounty,
		)
		app.sendWorkflowUnclaimedExpiredEmail(notice)
	}
}

func (a *AppService) sendWorkflowUnclaimedExpiredEmail(notice structs.WorkflowUnclaimedExpiryNotice) {
	toEmail := strings.TrimSpace(notice.ProposerEmail)
	if toEmail == "" {
		return
	}
	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	title := "Workflow Expired"
	startLabel := ""
	if notice.StartAt > 0 {
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			loc = time.UTC
		}
		startLabel = time.Unix(notice.StartAt, 0).In(loc).Format("January 2, 2006")
	}
	subtitle := fmt.Sprintf(
		"Nobody claimed this workflow within %d days of its start date, so it has expired and its funds have been returned to the faucet. To run it again, submit it as a new proposal.",
		db.UnclaimedWorkflowExpiryDays,
	)
	htmlContent := utils.BuildStyledEmail(
		title,
		subtitle,
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:160px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Start date</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Returned to the faucet</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%d SFLUV</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
</table>`,
			utils.EscapeEmailHTML(notice.WorkflowTitle),
			utils.EscapeEmailHTML(startLabel),
			notice.ReleasedBounty,
			utils.EscapeEmailHTML(notice.WorkflowId),
		),
	)

	if err := emailSender.SendEmail(toEmail, "Proposer", title, htmlContent, utils.NotificationFromEmail(), "SFLuv Workflows"); err != nil {
		a.logger.Logf("error sending unclaimed workflow expiry email for %s: %s", notice.WorkflowId, err)
	}
}
