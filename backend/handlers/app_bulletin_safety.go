package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// --- Blocks ---------------------------------------------------------------------

func (a *AppService) BlockUser(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	var req structs.BlockRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil || strings.TrimSpace(req.UserId) == "" || req.UserId == userId {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the request.")
		return
	}
	active, err := a.db.UserIsActive(r.Context(), req.UserId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !active {
		bulletinError(w, http.StatusNotFound, "not_found", "That person could not be found.")
		return
	}
	if err := a.db.BlockUser(r.Context(), userId, req.UserId); err != nil {
		a.logger.Logf("error blocking %s for %s: %s", req.UserId, userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) UnblockUser(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if err := a.db.UnblockUser(r.Context(), userId, userIdParam(r)); err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) ListBlockedUsers(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	ids, err := a.db.GetBlockedUserIds(r.Context(), userId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	people, err := a.db.GetPublicUsers(r.Context(), ids)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := []structs.PublicUser{}
	for _, id := range ids {
		if p, ok := people[id]; ok {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Reports --------------------------------------------------------------------

// ReportContent files a report on a post, a message, or a person. The item
// disappears from the reporter's own feed immediately. Three distinct reports
// on a post hide it from everyone until someone reviews it. Every report is
// emailed to the alerts inbox; the published commitment is a review within
// 24 hours.
func (a *AppService) ReportContent(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	var req structs.ContentReportRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<13)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the report.")
		return
	}
	req.TargetId = strings.TrimSpace(req.TargetId)
	req.Details = strings.TrimSpace(req.Details)
	if !structs.ContentReportReasons[req.Reason] {
		bulletinError(w, http.StatusBadRequest, "bad_reason", "Choose a reason for the report.")
		return
	}
	if utf8.RuneCountInString(req.Details) > 1000 {
		bulletinError(w, http.StatusBadRequest, "too_long", "Details can be up to 1,000 characters.")
		return
	}

	ctx := r.Context()
	notFound := func() { bulletinError(w, http.StatusNotFound, "not_found", "That could not be found.") }

	// Reporting twice is harmless, even if the first reports have since hidden
	// the post: same answer, no second email.
	if already, err := a.db.HasReported(ctx, userId, req.TargetType, req.TargetId); err == nil && already {
		writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
		return
	}

	var reportedUser string
	var postId int64
	var excerpt string // what was reported, for the alert email
	switch req.TargetType {
	case "post":
		id, err := strconv.ParseInt(req.TargetId, 10, 64)
		if err != nil {
			notFound()
			return
		}
		post, err := a.db.GetBulletinPost(ctx, id, userId)
		if err != nil || post.AuthorId == userId {
			notFound()
			return
		}
		reportedUser, postId = post.AuthorId, id
		excerpt = post.Title
		if post.Body != "" {
			excerpt += "\n\n" + post.Body
		}
	case "message":
		id, err := strconv.ParseInt(req.TargetId, 10, 64)
		if err != nil {
			notFound()
			return
		}
		visible, err := a.db.UserCanSeeMessage(ctx, id, userId)
		if err != nil || !visible {
			notFound()
			return
		}
		msg, err := a.db.GetMessageAny(ctx, id)
		if err != nil || msg.SenderId == userId {
			notFound()
			return
		}
		reportedUser = msg.SenderId
		excerpt = msg.Body
	case "user":
		if req.TargetId == userId {
			notFound()
			return
		}
		active, err := a.db.UserIsActive(ctx, req.TargetId)
		if err != nil || !active {
			notFound()
			return
		}
		reportedUser = req.TargetId
	default:
		bulletinError(w, http.StatusBadRequest, "bad_target", "That can't be reported.")
		return
	}

	openCount, err := a.db.CreateContentReport(ctx, userId, &req, reportedUser)
	if errors.Is(err, db.ErrAlreadyReported) {
		// Reporting twice is harmless: same answer, no second email.
		writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
		return
	}
	if err != nil {
		a.logger.Logf("error saving report from %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	autoHidden := false
	if req.TargetType == "post" && openCount >= db.BulletinAutoHideReports {
		if err := a.db.SetBulletinPostStatus(ctx, postId, structs.BulletinStatusActive, structs.BulletinStatusHidden); err != nil {
			a.logger.Logf("error auto-hiding post %d: %s", postId, err)
		} else {
			autoHidden = true
		}
	}

	a.sendBulletinReportAlert(bulletinReportAlert{
		Report:         req,
		ReporterId:     userId,
		ReportedUserId: reportedUser,
		Excerpt:        excerpt,
		OpenReports:    openCount,
		AutoHidden:     autoHidden,
		ReportedAt:     time.Now().UTC(),
	})
	writeJSON(w, http.StatusCreated, map[string]string{"status": "received"})
}

// defaultBulletinAlertEmail is the shared alerts inbox (see the dev
// requirements doc, "Better admin alerts"). Reports go there rather than to an
// admin-panel queue; a Telegram bot watching that inbox is how the team will
// see and act on them. BULLETIN_ALERT_EMAIL overrides it.
const defaultBulletinAlertEmail = "alerts@sfluv.org"

type bulletinReportAlert struct {
	Report         structs.ContentReportRequest
	ReporterId     string
	ReportedUserId string
	Excerpt        string
	OpenReports    int
	AutoHidden     bool
	ReportedAt     time.Time
}

// sendBulletinReportAlert emails one report to the alerts inbox.
//
// The email is written for two readers. People get a short summary. A bot gets
// a fixed subject ("[SFLuv alert] bulletin_report ...") and a block of
// "key: value" lines between ---BEGIN SFLUV ALERT--- and ---END SFLUV ALERT---
// markers, one field per line, stable names, so it can parse the report
// without scraping prose. Acting on a report is POST
// /admin/bulletin/reports/resolve with the target_type and target_id below.
func (a *AppService) sendBulletinReportAlert(alert bulletinReportAlert) {
	go func() {
		to := strings.TrimSpace(os.Getenv("BULLETIN_ALERT_EMAIL"))
		if to == "" {
			to = defaultBulletinAlertEmail
		}
		sender := utils.NewEmailSender()
		if sender == nil {
			a.logger.Logf("bulletin report %s %s not emailed: email sender is not configured", alert.Report.TargetType, alert.Report.TargetId)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		names, _ := a.db.GetPublicUsers(ctx, []string{alert.ReporterId, alert.ReportedUserId})
		reportedEmail := ""
		if summary, err := a.db.GetReportedUserSummary(ctx, alert.ReportedUserId); err == nil {
			reportedEmail = summary.ContactEmail
		}

		// One field per line; values never contain a newline.
		oneLine := func(s string) string { return strings.Join(strings.Fields(s), " ") }
		fields := [][2]string{
			{"alert_type", "bulletin_report"},
			{"target_type", alert.Report.TargetType},
			{"target_id", alert.Report.TargetId},
			{"reason", alert.Report.Reason},
			{"open_reports", strconv.Itoa(alert.OpenReports)},
			{"auto_hidden", strconv.FormatBool(alert.AutoHidden)},
			{"reported_user_id", alert.ReportedUserId},
			{"reported_user_name", oneLine(names[alert.ReportedUserId].PublicName)},
			{"reported_user_email", reportedEmail},
			{"reporter_user_id", alert.ReporterId},
			{"reporter_name", oneLine(names[alert.ReporterId].PublicName)},
			{"reported_at", alert.ReportedAt.Format(time.RFC3339)},
			{"details", oneLine(alert.Report.Details)},
		}
		var block strings.Builder
		block.WriteString("---BEGIN SFLUV ALERT---\n")
		for _, f := range fields {
			block.WriteString(f[0] + ": " + f[1] + "\n")
		}
		block.WriteString("---END SFLUV ALERT---")

		what := map[string]string{"post": "a Bulletin post", "message": "a message", "user": "a person"}[alert.Report.TargetType]
		reason := strings.ReplaceAll(alert.Report.Reason, "_", " ")
		hidden := ""
		if alert.AutoHidden {
			hidden = `<p style="font-size:13px;color:#b91c1c;">This post has enough reports that it is now hidden from everyone until it's reviewed.</p>`
		}
		excerpt := ""
		if strings.TrimSpace(alert.Excerpt) != "" {
			excerpt = fmt.Sprintf(`<p style="font-size:13px;color:#374151;white-space:pre-wrap;border-left:3px solid #e5e7eb;padding-left:10px;">%s</p>`,
				html.EscapeString(truncateRunes(alert.Excerpt, 800)))
		}
		details := ""
		if alert.Report.Details != "" {
			details = fmt.Sprintf(`<p style="font-size:13px;color:#374151;">Reporter's note: %s</p>`, html.EscapeString(alert.Report.Details))
		}
		body := fmt.Sprintf(`
<p style="font-size:14px;color:#111827;">%s reported %s by %s for <strong>%s</strong>. Open reports on it: %d.</p>
%s%s%s
<p style="font-size:13px;color:#374151;">Please review within 24 hours.</p>
<pre style="font-size:12px;color:#374151;background:#f9fafb;padding:10px;">%s</pre>`,
			html.EscapeString(names[alert.ReporterId].PublicName), what,
			html.EscapeString(names[alert.ReportedUserId].PublicName), html.EscapeString(reason),
			alert.OpenReports, hidden, excerpt, details, html.EscapeString(block.String()))

		subject := fmt.Sprintf("[SFLuv alert] bulletin_report %s %s (%s)", alert.Report.TargetType, alert.Report.TargetId, alert.Report.Reason)
		content := utils.BuildStyledEmail("Bulletin report", "Someone reported something on the Bulletin or in Messages.", body)
		if err := sender.SendEmail(to, "SFLuv alerts", subject, content, utils.NotificationFromEmail(), "SFLuv Bulletin"); err != nil {
			a.logger.Logf("error emailing bulletin report alert: %s", err)
		}
	}()
}

// --- Moderation API ---------------------------------------------------------------
//
// There is deliberately no admin-panel page for reports. They are handled from
// the alerts inbox, through a Telegram bot that watches it. These two admin
// endpoints are that bot's API (withAdmin accepts X-Admin-Key for scripts).

// AdminListBulletinReports lists every target with open reports, oldest first,
// with the reported content and the person's history.
func (a *AppService) AdminListBulletinReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targets, err := a.db.ListOpenReportTargets(ctx)
	if err != nil {
		a.logger.Logf("error loading report queue: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	out := []structs.AdminBulletinReport{}
	for _, t := range targets {
		item := structs.AdminBulletinReport{
			TargetType:   t.TargetType,
			TargetId:     t.TargetId,
			ReportCount:  t.Count,
			FirstAt:      t.FirstAt,
			PriorActions: []structs.AdminModerationRow{},
		}
		if item.Reports, err = a.db.GetOpenReportsForTarget(ctx, t.TargetType, t.TargetId); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch t.TargetType {
		case "post":
			if id, err := strconv.ParseInt(t.TargetId, 10, 64); err == nil {
				if row, err := a.db.GetBulletinPostAny(ctx, id); err == nil {
					if posts, err := a.buildBulletinPosts(ctx, "", []*structs.BulletinPostRow{row}, false); err == nil && len(posts) > 0 {
						item.Post = posts[0]
					}
				}
			}
		case "message":
			if id, err := strconv.ParseInt(t.TargetId, 10, 64); err == nil {
				if msg, err := a.db.GetMessageAny(ctx, id); err == nil {
					item.Message = msg
				}
			}
		}
		if t.ReportedUserId != nil {
			if summary, err := a.db.GetReportedUserSummary(ctx, *t.ReportedUserId); err == nil {
				item.ReportedUser = summary
			}
			if history, err := a.db.GetModerationHistory(ctx, *t.ReportedUserId); err == nil {
				item.PriorActions = history
			}
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// AdminResolveBulletinReport closes every open report on a target with one
// action. Actions: dismiss, remove_post, remove_message, warn, suspend,
// unsuspend. Suspension only affects the Bulletin and Messages; the person's
// wallet and SFLUV keep working.
func (a *AppService) AdminResolveBulletinReport(w http.ResponseWriter, r *http.Request) {
	adminId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	var req structs.AdminModerationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<13)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the request.")
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	ctx := r.Context()

	// Find who the target belongs to, so warnings and suspensions land on the
	// right person even if the request omits it.
	targetUser := strings.TrimSpace(req.TargetUserId)
	switch req.TargetType {
	case "post":
		if id, err := strconv.ParseInt(req.TargetId, 10, 64); err == nil {
			if post, err := a.db.GetBulletinPostAny(ctx, id); err == nil {
				targetUser = post.AuthorId
			}
		}
	case "message":
		if id, err := strconv.ParseInt(req.TargetId, 10, 64); err == nil {
			if msg, err := a.db.GetMessageAny(ctx, id); err == nil {
				targetUser = msg.SenderId
			}
		}
	case "user":
		targetUser = req.TargetId
	default:
		bulletinError(w, http.StatusBadRequest, "bad_target", "Unknown report target.")
		return
	}

	resolution := "actioned"
	switch req.Action {
	case "dismiss":
		resolution = "dismissed"
		if req.TargetType == "post" {
			if id, err := strconv.ParseInt(req.TargetId, 10, 64); err == nil {
				_ = a.db.SetBulletinPostStatus(ctx, id, structs.BulletinStatusHidden, structs.BulletinStatusActive)
			}
		}
	case "remove_post":
		id, err := strconv.ParseInt(req.TargetId, 10, 64)
		if req.TargetType != "post" || err != nil {
			bulletinError(w, http.StatusBadRequest, "bad_action", "Only posts can be removed with this action.")
			return
		}
		if err := a.db.RemoveBulletinPost(ctx, id, "", "moderation"); err != nil && !errors.Is(err, db.ErrBulletinNotFound) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		a.notifyModeration(targetUser, "Your Bulletin post was removed because it broke the community rules.")
	case "remove_message":
		id, err := strconv.ParseInt(req.TargetId, 10, 64)
		if req.TargetType != "message" || err != nil {
			bulletinError(w, http.StatusBadRequest, "bad_action", "Only messages can be removed with this action.")
			return
		}
		if err := a.db.RemoveMessage(ctx, id); err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case "warn":
		if targetUser == "" {
			bulletinError(w, http.StatusBadRequest, "bad_action", "There is no one to warn.")
			return
		}
		a.notifyModeration(targetUser, "A reminder from SFLuv: something you shared broke the Bulletin community rules. Repeated issues can pause your access.")
	case "suspend":
		if targetUser == "" {
			bulletinError(w, http.StatusBadRequest, "bad_action", "There is no one to suspend.")
			return
		}
		days := req.SuspendDays
		if days <= 0 || days > 3650 {
			days = 3650 // "until further notice"
		}
		until := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		if err := a.db.SetBulletinSuspension(ctx, targetUser, &until, req.Note); err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		a.notifyModeration(targetUser, "Your access to the Bulletin and Messages has been paused. Your wallet still works. Contact admin@sfluv.org with questions.")
	case "unsuspend":
		if targetUser == "" {
			bulletinError(w, http.StatusBadRequest, "bad_action", "There is no one to restore.")
			return
		}
		if err := a.db.SetBulletinSuspension(ctx, targetUser, nil, ""); err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	default:
		bulletinError(w, http.StatusBadRequest, "bad_action", "Unknown action.")
		return
	}

	reportIds, err := a.db.ResolveReportsForTarget(ctx, req.TargetType, req.TargetId, resolution, adminId)
	if err != nil {
		a.logger.Logf("error resolving reports on %s %s: %s", req.TargetType, req.TargetId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var firstReport *int64
	if len(reportIds) > 0 {
		firstReport = &reportIds[0]
	}
	if err := a.db.RecordModerationAction(ctx, adminId, targetUser, firstReport, req.Action, req.Note); err != nil {
		a.logger.Logf("error recording moderation action: %s", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"resolved_reports": len(reportIds)})
}

// notifyModeration tells a user, by push, that an admin acted on something of
// theirs. Best effort: a user with no registered device simply isn't pushed.
func (a *AppService) notifyModeration(userId, body string) {
	if userId == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		a.recordNotification(ctx, userId, "bulletin_moderation", "SFLuv Bulletin", body, map[string]string{
			"type": "bulletin_moderation",
		})
		subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userId)
		if err != nil {
			return
		}
		seen := map[string]struct{}{}
		for _, s := range subscriptions {
			if s == nil || !s.Active || !s.DeviceRegistered {
				continue
			}
			token := strings.TrimSpace(s.Token)
			if _, dup := seen[token]; dup || token == "" {
				continue
			}
			seen[token] = struct{}{}
			_, _ = sendExpoPushNotification(ctx, token, "SFLuv Bulletin", body, map[string]string{"type": "bulletin_moderation"})
		}
	}()
}
