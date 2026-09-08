package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

const (
	signupRateWindow     = 10 * time.Minute
	signupMaxPerIP       = 10
	signupMaxPerEmail    = 3
	volunteerListActive  = "active"
	volunteerListPending = "pending_confirmation"
	volunteerListNone    = "none"
)

type volunteerSignupRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// Both spellings are accepted: mobile proposed volunteer_list_opt_in and the
	// original contract said marketing_opt_in. Taking both costs one field and
	// removes a whole class of silent no-op.
	VolunteerListOptIn *bool `json:"volunteer_list_opt_in,omitempty"`
	MarketingOptIn     *bool `json:"marketing_opt_in,omitempty"`
	// Honeypot: a real user never fills this, a naive bot fills everything.
	Website string `json:"website,omitempty"`
}

func (r *volunteerSignupRequest) optedIn() bool {
	if r.VolunteerListOptIn != nil {
		return *r.VolunteerListOptIn
	}
	if r.MarketingOptIn != nil {
		return *r.MarketingOptIn
	}
	return false
}

type volunteerSignupResponse struct {
	SignupId       string `json:"signup_id"`
	Status         string `json:"status"`
	SpotsRemaining int    `json:"spots_remaining"`
	VolunteerList  string `json:"volunteer_list"`
}

func writeSignupError(w http.ResponseWriter, status int, reason string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"reason": reason, "message": message})
}

// clientIPForRateLimit resolves the address to rate-limit on.
//
// X-Forwarded-For is trusted ONLY when the caller presents the shared proxy
// secret. sfluv.org proxies signups server-side, so without this every web
// signup would share one bucket; but trusting the header unconditionally would
// let anyone forge an address and evade the limit entirely. Vercel has no
// stable egress IPs, which is why this is a secret rather than an IP allowlist.
func clientIPForRateLimit(r *http.Request) string {
	expected := strings.TrimSpace(os.Getenv("VOLUNTEER_PROXY_KEY"))
	presented := strings.TrimSpace(r.Header.Get("X-SFLUV-Proxy-Key"))

	if expected != "" && presented == expected {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			first, _, _ := strings.Cut(forwarded, ",")
			if first = strings.TrimSpace(first); first != "" {
				return first
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// SignUpForVolunteerEvent is the one signup path for both clients: auth is
// optional. An authenticated caller's identity comes from their profile; an
// anonymous caller supplies it in the body.
func (s *BotService) SignUpForVolunteerEvent(w http.ResponseWriter, r *http.Request) {
	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var req volunteerSignupRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeSignupError(w, http.StatusBadRequest, "validation_error", "Could not read the signup.")
			return
		}
	}

	// Honeypot filled: accept the request so a bot cannot distinguish rejection
	// from success, but record nothing.
	if strings.TrimSpace(req.Website) != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(volunteerSignupResponse{
			Status: "confirmed", VolunteerList: volunteerListNone,
		})
		return
	}

	userDid := utils.GetDid(r)
	// Portal (anonymous) signups must confirm by email; an authenticated signup
	// is already tied to an account, so it is confirmed on the spot.
	params := &db.VolunteerSignupParams{EventId: eventId, Source: "web", RequireConfirmation: true}

	if userDid != nil {
		profile, err := s.appDb.GetUserById(r.Context(), *userDid)
		if err != nil || profile == nil {
			writeSignupError(w, http.StatusForbidden, "validation_error", "Could not load your account.")
			return
		}
		params.UserId = userDid
		params.Source = "mobile"
		params.RequireConfirmation = false
		if profile.Email != nil {
			params.Email = strings.TrimSpace(*profile.Email)
		}
		if profile.Name != nil {
			params.FirstName, params.LastName = splitContactName(*profile.Name)
		}
		if params.Email == "" {
			writeSignupError(w, http.StatusBadRequest, "validation_error", "Add an email to your account before signing up.")
			return
		}
	} else {
		params.Email = strings.TrimSpace(req.Email)
		params.FirstName = strings.TrimSpace(req.FirstName)
		params.LastName = strings.TrimSpace(req.LastName)

		if _, err := mail.ParseAddress(params.Email); err != nil {
			writeSignupError(w, http.StatusBadRequest, "validation_error", "Enter a valid email address.")
			return
		}
		if params.FirstName == "" || params.LastName == "" {
			writeSignupError(w, http.StatusBadRequest, "validation_error", "First and last name are required.")
			return
		}

		// Rate limiting applies to the anonymous path only: an authenticated
		// caller is already identified and bounded by the per-event unique index.
		ip := clientIPForRateLimit(r)
		since := time.Now().Add(-signupRateWindow).Unix()
		byIP, byEmail, err := s.db.CountRecentSignupAttempts(r.Context(), ip, params.Email, since)
		if err != nil {
			fmt.Printf("error counting signup attempts: %s\n", err)
		} else if byIP >= signupMaxPerIP || byEmail >= signupMaxPerEmail {
			writeSignupError(w, http.StatusTooManyRequests, "rate_limited", "Too many signups just now. Please try again shortly.")
			return
		}
		if err := s.db.RecordSignupAttempt(r.Context(), ip, params.Email); err != nil {
			fmt.Printf("error recording signup attempt: %s\n", err)
		}
	}

	result, err := s.db.CreateVolunteerSignup(r.Context(), params)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrEventNotFound):
			writeSignupError(w, http.StatusNotFound, "not_found", "That event no longer exists.")
		case errors.Is(err, db.ErrSignupNotInternal):
			writeSignupError(w, http.StatusBadRequest, "not_internal", "This event does not take signups here.")
		case errors.Is(err, db.ErrSignupEventClosed):
			writeSignupError(w, http.StatusGone, "closed", "Signups for this event are closed.")
		case errors.Is(err, db.ErrSignupEventFull):
			writeSignupError(w, http.StatusConflict, "full", "This event is full.")
		case errors.Is(err, db.ErrAlreadySignedUp):
			writeSignupError(w, http.StatusConflict, "already_signed_up", "You are already signed up for this event.")
		default:
			fmt.Printf("error creating volunteer signup: %s\n", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// Volunteer list membership. An authenticated user is auto-confirmed (PJ's
	// ruling); an anonymous form entry stays pending until the double opt-in
	// link is followed, because that form is the spam vector.
	listState := volunteerListNone
	if req.optedIn() {
		status, _, err := s.appDb.UpsertVolunteerListSubscription(
			r.Context(), params.Email, params.FirstName, params.LastName, eventId, userDid != nil,
		)
		if err != nil {
			fmt.Printf("error recording volunteer list opt-in: %s\n", err)
		} else if status == db.VolunteerListActive {
			listState = volunteerListActive
		} else {
			listState = volunteerListPending
		}
	}

	// Load the event only when a confirmation email is actually going out.
	eventTitle, eventStartAt := "your volunteer event", int64(0)
	if result.ConfirmToken != "" {
		if row, err := s.db.GetPublicVolunteerEvent(r.Context(), eventId); err == nil && row != nil {
			eventTitle, eventStartAt = row.Title, row.StartAt
		}
	}

	signupStatus := "confirmed"
	if result.ConfirmToken != "" {
		signupStatus = "pending_confirmation"
		if s.app != nil {
			go s.app.sendVolunteerSignupConfirmationEmail(
				params.Email, params.FirstName, eventTitle, eventStartAt, result.ConfirmToken,
			)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(volunteerSignupResponse{
		SignupId:       result.SignupId,
		Status:         signupStatus,
		SpotsRemaining: result.SpotsRemaining,
		VolunteerList:  listState,
	})
}

func splitContactName(full string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(full))
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// CancelVolunteerEventSignup frees the caller's spot. Cancelling touches no
// faucet accounting: the allocation is reserved per event, not per signup.
func (s *BotService) CancelVolunteerEventSignup(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventId := strings.TrimSpace(r.PathValue("id"))
	if eventId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cancelled, err := s.db.CancelVolunteerSignup(r.Context(), eventId, *userDid)
	if err != nil {
		fmt.Printf("error cancelling volunteer signup: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !cancelled {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMyVolunteerEvents returns the caller's signed-up events in the same
// envelope as the list, so clients reuse one mapper.
func (s *BotService) GetMyVolunteerEvents(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	ids, err := s.db.GetMyVolunteerEventIds(r.Context(), *userDid)
	if err != nil {
		fmt.Printf("error listing my volunteer events: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	rows, err := s.db.GetVolunteerEventsByIds(r.Context(), ids)
	if err != nil {
		fmt.Printf("error loading my volunteer events: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	eventCtx := s.buildVolunteerEventContext(r, rows)
	events := make([]*structs.VolunteerEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, s.mapVolunteerEvent(row, eventCtx))
	}

	setVolunteerCacheHeaders(w, true)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.VolunteerEventsResponse{
		Events: events, Page: 0, Count: len(events), Total: len(events), HasMore: false,
	})
}

type volunteerListTokenRequest struct {
	Token string `json:"token"`
}

// GetVolunteerListTokenState is the READ half of the token flow: it renders the
// landing page without mutating anything. Mail clients and corporate link
// scanners prefetch URLs in email, so a GET that mutated would unsubscribe — or
// silently complete a double opt-in for — people who never clicked.
func (a *AppService) GetVolunteerListTokenState(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	confirm := strings.HasSuffix(r.URL.Path, "/confirm")

	email, status, err := a.db.PeekVolunteerListByToken(r.Context(), token, confirm)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error reading volunteer list token: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"email": email, "status": status})
}

// PostVolunteerListTokenAction performs the mutation, on an explicit POST.
func (a *AppService) PostVolunteerListTokenAction(w http.ResponseWriter, r *http.Request) {
	confirm := strings.HasSuffix(r.URL.Path, "/confirm")

	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var req volunteerListTokenRequest
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	if req.Token == "" {
		req.Token = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	email, status, err := a.db.SetVolunteerListStateByToken(r.Context(), req.Token, confirm)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error applying volunteer list token action: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"email": email, "status": status})
}

type reminderPreferencePayload struct {
	Enabled     bool `json:"enabled"`
	HoursBefore int  `json:"hours_before"`
}

func (a *AppService) GetVolunteerReminderPreferences(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	enabled, hoursBefore, err := a.db.GetVolunteerReminderPreference(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading reminder preferences for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(reminderPreferencePayload{Enabled: enabled, HoursBefore: hoursBefore})
}

func (a *AppService) SetVolunteerReminderPreferences(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req reminderPreferencePayload
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Range-checked here rather than trusting the client, since this value
	// drives when a push actually fires.
	if req.HoursBefore < db.VolunteerReminderMinHours || req.HoursBefore > db.VolunteerReminderMaxHours {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("hours_before must be between 1 and 168"))
		return
	}

	if err := a.db.SetVolunteerReminderPreference(r.Context(), *userDid, req.Enabled, req.HoursBefore); err != nil {
		a.logger.Logf("error saving reminder preferences for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(req)
}
