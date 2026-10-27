package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

const (
	bulletinPostLifetime       = 30 * 24 * time.Hour
	bulletinMaxPostsPerDay     = 5
	bulletinDefaultRadiusMiles = 1.0
	bulletinMinRadiusMiles     = 0.25
	bulletinMaxRadiusMiles     = 10.0
	bulletinFeedPageSize       = 20
	bulletinMaxPhotos          = 6
	// Matches the mobile app's workflow-photo cap: the picker compresses at
	// quality 0.45, which keeps ordinary phone photos well under this.
	bulletinMaxPhotoBytes = 4 << 20
	// A ceiling on asking prices, so a typo cannot list a chair for 1,000,000.
	bulletinMaxPriceSfluv = 10000
)

var bulletinHandlePattern = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)

// bulletinError writes the error shape the mobile app shows: a sentence for
// people, and a stable code for the app to branch on when it needs to.
//
// Never use 401 or 403 for a business rule here (suspended, blocked, not your
// trade): the mobile client treats any 401/403 as a dead session and asks the
// person to sign in again. Use 409, 423, or 429 instead.
func bulletinError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

func bulletinViewer(w http.ResponseWriter, r *http.Request) (string, bool) {
	userDid := utils.GetDid(r)
	if userDid == nil || *userDid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return "", false
	}
	return *userDid, true
}

// --- Profile ------------------------------------------------------------------

// suggestPublicName turns "Maria Gonzalez" into "Maria G.", the default shown
// the first time someone posts or messages. They can change it.
func suggestPublicName(contactName string) string {
	parts := strings.Fields(contactName)
	if len(parts) == 0 {
		return ""
	}
	first := parts[0]
	if len(parts) == 1 {
		return truncateRunes(first, 40)
	}
	last, _ := utf8.DecodeRuneInString(parts[len(parts)-1])
	return truncateRunes(fmt.Sprintf("%s %s.", first, strings.ToUpper(string(last))), 40)
}

func (a *AppService) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	profile, err := a.db.GetUserProfile(r.Context(), userId)
	if err != nil {
		a.logger.Logf("error loading profile for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if profile == nil {
		contactName, err := a.db.GetUserContactName(r.Context(), userId)
		if err != nil {
			a.logger.Logf("error loading contact name for %s: %s", userId, err)
		}
		profile = &structs.UserProfile{
			UserId:              userId,
			SuggestedPublicName: suggestPublicName(contactName),
		}
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *AppService) PutMyProfile(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	var req structs.UserProfileRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the request.")
		return
	}

	req.PublicName = strings.Join(strings.Fields(req.PublicName), " ")
	if n := utf8.RuneCountInString(req.PublicName); n < 1 || n > 40 {
		bulletinError(w, http.StatusBadRequest, "invalid_name", "Your name needs to be between 1 and 40 characters.")
		return
	}
	if req.Handle != nil {
		h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(*req.Handle), "@"))
		if h == "" {
			req.Handle = nil
		} else if !bulletinHandlePattern.MatchString(h) {
			bulletinError(w, http.StatusBadRequest, "invalid_handle", "Handles are 3 to 20 lowercase letters, numbers, or underscores.")
			return
		} else {
			req.Handle = &h
		}
	}

	profile, err := a.db.UpsertUserProfile(r.Context(), userId, &req)
	if errors.Is(err, db.ErrHandleTaken) {
		bulletinError(w, http.StatusConflict, "handle_taken", "That handle is already taken. Try another.")
		return
	}
	if err != nil {
		a.logger.Logf("error saving profile for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

// requireBulletinParticipant checks the two things every write on the Bulletin
// and in Messages needs: a public name to show, and no active suspension.
// It writes the response and returns false when either is missing.
func (a *AppService) requireBulletinParticipant(w http.ResponseWriter, r *http.Request, userId string) bool {
	profile, err := a.db.GetUserProfile(r.Context(), userId)
	if err != nil {
		a.logger.Logf("error loading profile for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	if profile == nil {
		bulletinError(w, http.StatusConflict, "profile_required", "Choose how neighbors will see your name first.")
		return false
	}
	if profile.SuspendedUntil != nil && profile.SuspendedUntil.After(time.Now()) {
		bulletinError(w, http.StatusLocked, "suspended", "Your access to the Bulletin and Messages is paused. Contact admin@sfluv.org with questions.")
		return false
	}
	return true
}

// --- Building responses -------------------------------------------------------

// buildBulletinPosts turns stored rows into what the app shows: author badges,
// photos, meetup spot names, and linked events. When dropUnlinkable is set (the
// feed), a notice whose event was cancelled or has ended is left out.
func (a *AppService) buildBulletinPosts(ctx context.Context, viewerId string, rows []*structs.BulletinPostRow, dropUnlinkable bool) ([]*structs.BulletinPost, error) {
	authorIds := []string{}
	postIds := []int64{}
	meetupIds := []int{}
	seenAuthor := map[string]bool{}
	for _, row := range rows {
		if !seenAuthor[row.AuthorId] {
			seenAuthor[row.AuthorId] = true
			authorIds = append(authorIds, row.AuthorId)
		}
		postIds = append(postIds, row.Id)
		if row.MeetupLocationId != nil {
			meetupIds = append(meetupIds, *row.MeetupLocationId)
		}
	}

	authors, err := a.db.GetPublicUsers(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	photos, err := a.db.GetBulletinPhotosForPosts(ctx, postIds)
	if err != nil {
		return nil, err
	}
	meetups, err := a.db.GetApprovedLocationNames(ctx, meetupIds)
	if err != nil {
		return nil, err
	}

	out := make([]*structs.BulletinPost, 0, len(rows))
	for _, row := range rows {
		post := &structs.BulletinPost{
			Id:           row.Id,
			Author:       authors[row.AuthorId],
			Category:     row.Category,
			Title:        row.Title,
			Body:         row.Body,
			PriceSfluv:   displayBulletinPrice(row.PriceSfluv),
			AcceptsSfluv: row.AcceptsSfluv,
			AcceptsCash:  row.AcceptsCash,
			AreaLabel:    row.AreaLabel,
			Status:       row.Status,
			ExpiresAt:    row.ExpiresAt,
			CreatedAt:    row.CreatedAt,
			Photos:       photos[row.Id],
			IsMine:       row.AuthorId == viewerId,
		}
		if post.Photos == nil {
			post.Photos = []structs.BulletinPhoto{}
		}
		if row.DistanceMiles != nil {
			d := roundBulletinDistance(*row.DistanceMiles)
			post.DistanceMiles = &d
		}
		if row.MeetupLocationId != nil {
			if name, ok := meetups[*row.MeetupLocationId]; ok {
				post.MeetupLocation = &structs.BulletinMeetupLocation{Id: *row.MeetupLocationId, Name: name}
			}
		}
		if row.LinkedEventId != nil {
			event, live := a.lookupLinkedEvent(ctx, *row.LinkedEventId)
			if !live && dropUnlinkable {
				continue
			}
			post.LinkedEvent = event
		}
		out = append(out, post)
	}
	return out, nil
}

// lookupLinkedEvent resolves a Participate event for a notice. live is false
// when the event is gone, cancelled, or over.
func (a *AppService) lookupLinkedEvent(ctx context.Context, eventId string) (*structs.BulletinLinkedEvent, bool) {
	if a.bot == nil || a.bot.db == nil {
		return nil, false
	}
	row, err := a.bot.db.GetPublicVolunteerEvent(ctx, eventId)
	if err != nil || row == nil {
		return nil, false
	}
	end := volunteerEventEnd(row)
	event := &structs.BulletinLinkedEvent{
		Id:      row.Id,
		Title:   row.Title,
		StartAt: row.StartAt,
		EndAt:   end.Unix(),
	}
	live := row.CancelledAt == nil && row.ReviewStatus == "approved" && end.After(time.Now())
	return event, live
}

// volunteerEventEnd is when a volunteer event finishes. For volunteer events
// the events.expiration column holds the event's end time (the QR codes have
// their own qr_expires_at). An event saved without a usable end is treated as
// running until midnight on its start day, in its own time zone.
func volunteerEventEnd(row *db.VolunteerEventRow) time.Time {
	if row.Expiration > row.StartAt {
		return time.Unix(row.Expiration, 0)
	}
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc, _ = time.LoadLocation("America/Los_Angeles")
	}
	start := time.Unix(row.StartAt, 0).In(loc)
	return time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, loc)
}

// --- Feed and posts ---------------------------------------------------------------

func (a *AppService) GetBulletinFeed(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()

	lat, errLat := strconv.ParseFloat(q.Get("lat"), 64)
	lng, errLng := strconv.ParseFloat(q.Get("lng"), 64)
	if errLat != nil || errLng != nil || !validBulletinCoordinate(lat, lng) {
		bulletinError(w, http.StatusBadRequest, "location_required", "Your location is needed to show posts near you.")
		return
	}

	radius := bulletinDefaultRadiusMiles
	if raw := q.Get("radius_mi"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			bulletinError(w, http.StatusBadRequest, "bad_radius", "That distance isn't valid.")
			return
		}
		radius = parsed
	}
	if radius < bulletinMinRadiusMiles {
		radius = bulletinMinRadiusMiles
	}
	if radius > bulletinMaxRadiusMiles {
		radius = bulletinMaxRadiusMiles
	}

	category := q.Get("category")
	if category != "" && category != structs.BulletinCategoryFree &&
		category != structs.BulletinCategoryForSale && category != structs.BulletinCategoryNotice {
		bulletinError(w, http.StatusBadRequest, "bad_category", "That category doesn't exist.")
		return
	}

	afterDistance, afterId, err := db.ParseBulletinCursor(q.Get("cursor"))
	if err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_cursor", "That page link is no longer valid.")
		return
	}

	rows, hasMore, err := a.db.GetBulletinFeed(r.Context(), &structs.BulletinFeedRequest{
		ViewerId:      userId,
		Lat:           lat,
		Lng:           lng,
		RadiusMiles:   radius,
		Category:      category,
		AfterDistance: afterDistance,
		AfterId:       afterId,
		Limit:         bulletinFeedPageSize,
	})
	if err != nil {
		a.logger.Logf("error loading bulletin feed for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var next *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cursor := db.FormatBulletinCursor(*last.DistanceMiles, last.Id)
		next = &cursor
	}

	posts, err := a.buildBulletinPosts(r.Context(), userId, rows, true)
	if err != nil {
		a.logger.Logf("error building bulletin feed for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, structs.BulletinFeedPage{Posts: posts, NextCursor: next})
}

func bulletinPostIdFromPath(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

func (a *AppService) GetBulletinPost(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	postId, ok := bulletinPostIdFromPath(r)
	if !ok {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return
	}

	row, err := a.db.GetBulletinPost(r.Context(), postId, userId)
	if errors.Is(err, db.ErrBulletinNotFound) {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return
	}
	if err != nil {
		a.logger.Logf("error loading bulletin post %d: %s", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Distance is optional on the detail view: the app sends its position when
	// it has one.
	q := r.URL.Query()
	if lat, err1 := strconv.ParseFloat(q.Get("lat"), 64); err1 == nil {
		if lng, err2 := strconv.ParseFloat(q.Get("lng"), 64); err2 == nil && validBulletinCoordinate(lat, lng) {
			d := haversineMiles(lat, lng, row.ApproxLat, row.ApproxLng)
			row.DistanceMiles = &d
		}
	}

	posts, err := a.buildBulletinPosts(r.Context(), userId, []*structs.BulletinPostRow{row}, false)
	if err != nil || len(posts) == 0 {
		a.logger.Logf("error building bulletin post %d: %v", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, posts[0])
}

func (a *AppService) GetMyBulletinPosts(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	rows, err := a.db.GetMyBulletinPosts(r.Context(), userId)
	if err != nil {
		a.logger.Logf("error loading bulletin posts for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	posts, err := a.buildBulletinPosts(r.Context(), userId, rows, false)
	if err != nil {
		a.logger.Logf("error building bulletin posts for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

// normalizeBulletinPrice validates an asking price and returns it in canonical
// form ("12" or "12.5"). Whole SFLUV with up to two decimals, above zero.
func normalizeBulletinPrice(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	rat, ok := new(big.Rat).SetString(raw)
	if !ok || rat.Sign() <= 0 {
		return "", false
	}
	if rat.Cmp(big.NewRat(bulletinMaxPriceSfluv, 1)) > 0 {
		return "", false
	}
	cents := new(big.Rat).Mul(rat, big.NewRat(100, 1))
	if !cents.IsInt() {
		return "", false
	}
	return strings.TrimRight(strings.TrimRight(rat.FloatString(2), "0"), "."), true
}

// validateBulletinPricing enforces the category rules the database also checks,
// so the person gets a readable message instead of a constraint error.
func validateBulletinPricing(category string, price *string, acceptsSfluv, acceptsCash bool) (*string, string, string) {
	if category != structs.BulletinCategoryForSale {
		return nil, "", ""
	}
	if price == nil {
		return nil, "price_required", "Add a price for items for sale."
	}
	normalized, ok := normalizeBulletinPrice(*price)
	if !ok {
		return nil, "bad_price", fmt.Sprintf("Prices must be between 0.01 and %d, with at most two decimal places.", bulletinMaxPriceSfluv)
	}
	if !acceptsSfluv && !acceptsCash {
		return nil, "payment_required", "Choose SFLUV, cash, or both."
	}
	return &normalized, "", ""
}

// displayBulletinPrice drops the zero padding the numeric column adds, so a
// price reads "40" or "40.5" rather than "40.00".
func displayBulletinPrice(price *string) *string {
	if price == nil {
		return nil
	}
	s := *price
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return &s
}

func cleanBulletinTitle(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (a *AppService) CreateBulletinPost(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if !a.requireBulletinParticipant(w, r, userId) {
		return
	}

	var req structs.BulletinPostRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the post.")
		return
	}

	switch req.Category {
	case structs.BulletinCategoryFree, structs.BulletinCategoryForSale, structs.BulletinCategoryNotice:
	default:
		bulletinError(w, http.StatusBadRequest, "bad_category", "Choose Free, For sale, or Neighborhood notice.")
		return
	}

	req.Title = cleanBulletinTitle(req.Title)
	if n := utf8.RuneCountInString(req.Title); n < 3 || n > 80 {
		bulletinError(w, http.StatusBadRequest, "bad_title", "Titles need to be between 3 and 80 characters.")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if utf8.RuneCountInString(req.Body) > 2000 {
		bulletinError(w, http.StatusBadRequest, "bad_body", "Descriptions can be up to 2,000 characters.")
		return
	}

	price, code, message := validateBulletinPricing(req.Category, req.PriceSfluv, req.AcceptsSfluv, req.AcceptsCash)
	if code != "" {
		bulletinError(w, http.StatusBadRequest, code, message)
		return
	}
	if req.Category != structs.BulletinCategoryForSale {
		req.AcceptsSfluv, req.AcceptsCash = false, false
	}

	if !validBulletinCoordinate(req.Lat, req.Lng) {
		bulletinError(w, http.StatusBadRequest, "location_required", "Your location is needed to post.")
		return
	}

	ctx := r.Context()
	if req.MeetupLocationId != nil {
		names, err := a.db.GetApprovedLocationNames(ctx, []int{*req.MeetupLocationId})
		if err != nil {
			a.logger.Logf("error checking meetup location: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, ok := names[*req.MeetupLocationId]; !ok {
			bulletinError(w, http.StatusBadRequest, "bad_meetup", "Meetup spots must be an SFLuv partner merchant.")
			return
		}
	}

	expiresAt := time.Now().Add(bulletinPostLifetime)
	if req.LinkedEventId != nil {
		if req.Category != structs.BulletinCategoryNotice {
			bulletinError(w, http.StatusBadRequest, "bad_event", "Only neighborhood notices can link an event.")
			return
		}
		event, live := a.lookupLinkedEvent(ctx, *req.LinkedEventId)
		if event == nil || !live {
			bulletinError(w, http.StatusBadRequest, "bad_event", "That event isn't available to link. It may be over or cancelled.")
			return
		}
		// A notice about an event goes away when the event ends.
		if end := time.Unix(event.EndAt, 0); end.Before(expiresAt) {
			expiresAt = end
		}
	}

	since := time.Now().Add(-24 * time.Hour)
	recent, err := a.db.CountRecentBulletinPosts(ctx, userId, since)
	if err != nil {
		a.logger.Logf("error counting recent posts for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if recent >= bulletinMaxPostsPerDay {
		bulletinError(w, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("You can post up to %d times a day. Try again tomorrow.", bulletinMaxPostsPerDay))
		return
	}

	lat, lng := snapBulletinCoordinate(req.Lat, req.Lng)
	postId, err := a.db.CreateBulletinPost(ctx, &db.NewBulletinPost{
		AuthorId:         userId,
		Category:         req.Category,
		Title:            req.Title,
		Body:             req.Body,
		PriceSfluv:       price,
		AcceptsSfluv:     req.AcceptsSfluv,
		AcceptsCash:      req.AcceptsCash,
		ApproxLat:        lat,
		ApproxLng:        lng,
		AreaLabel:        bulletinAreaLabel(lat, lng),
		MeetupLocationId: req.MeetupLocationId,
		LinkedEventId:    req.LinkedEventId,
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		a.logger.Logf("error creating bulletin post for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	row, err := a.db.GetBulletinPost(ctx, postId, userId)
	if err != nil {
		a.logger.Logf("error reloading bulletin post %d: %s", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	posts, err := a.buildBulletinPosts(ctx, userId, []*structs.BulletinPostRow{row}, false)
	if err != nil || len(posts) == 0 {
		a.logger.Logf("error building bulletin post %d: %v", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, posts[0])
}

// loadOwnBulletinPost loads a post the caller wrote, writing a 404 otherwise.
func (a *AppService) loadOwnBulletinPost(w http.ResponseWriter, r *http.Request, userId string) (*structs.BulletinPostRow, bool) {
	postId, ok := bulletinPostIdFromPath(r)
	if !ok {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return nil, false
	}
	row, err := a.db.GetBulletinPost(r.Context(), postId, userId)
	if errors.Is(err, db.ErrBulletinNotFound) || (err == nil && row.AuthorId != userId) {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return nil, false
	}
	if err != nil {
		a.logger.Logf("error loading bulletin post %d: %s", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	return row, true
}

func (a *AppService) UpdateBulletinPost(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if !a.requireBulletinParticipant(w, r, userId) {
		return
	}
	row, ok := a.loadOwnBulletinPost(w, r, userId)
	if !ok {
		return
	}

	var patch structs.BulletinPostPatch
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&patch); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the change.")
		return
	}
	if row.Status == structs.BulletinStatusHidden {
		bulletinError(w, http.StatusConflict, "under_review", "This post is being reviewed and can't be changed right now.")
		return
	}

	if patch.Title != nil {
		title := cleanBulletinTitle(*patch.Title)
		if n := utf8.RuneCountInString(title); n < 3 || n > 80 {
			bulletinError(w, http.StatusBadRequest, "bad_title", "Titles need to be between 3 and 80 characters.")
			return
		}
		row.Title = title
	}
	if patch.Body != nil {
		body := strings.TrimSpace(*patch.Body)
		if utf8.RuneCountInString(body) > 2000 {
			bulletinError(w, http.StatusBadRequest, "bad_body", "Descriptions can be up to 2,000 characters.")
			return
		}
		row.Body = body
	}
	if patch.PriceSfluv != nil {
		row.PriceSfluv = patch.PriceSfluv
	}
	if patch.AcceptsSfluv != nil {
		row.AcceptsSfluv = *patch.AcceptsSfluv
	}
	if patch.AcceptsCash != nil {
		row.AcceptsCash = *patch.AcceptsCash
	}
	price, code, message := validateBulletinPricing(row.Category, row.PriceSfluv, row.AcceptsSfluv, row.AcceptsCash)
	if code != "" {
		bulletinError(w, http.StatusBadRequest, code, message)
		return
	}
	row.PriceSfluv = price

	if patch.ClearMeetup {
		row.MeetupLocationId = nil
	} else if patch.MeetupLocationId != nil {
		names, err := a.db.GetApprovedLocationNames(r.Context(), []int{*patch.MeetupLocationId})
		if err != nil {
			a.logger.Logf("error checking meetup location: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, ok := names[*patch.MeetupLocationId]; !ok {
			bulletinError(w, http.StatusBadRequest, "bad_meetup", "Meetup spots must be an SFLuv partner merchant.")
			return
		}
		row.MeetupLocationId = patch.MeetupLocationId
	}

	if patch.Status != nil {
		next := *patch.Status
		switch {
		case next == structs.BulletinStatusActive:
		case next == structs.BulletinStatusSold && row.Category == structs.BulletinCategoryForSale:
		case next == structs.BulletinStatusClaimed && row.Category == structs.BulletinCategoryFree:
		default:
			bulletinError(w, http.StatusBadRequest, "bad_status", "That status doesn't apply to this post.")
			return
		}
		row.Status = next
	}

	if patch.Renew {
		if row.LinkedEventId != nil {
			bulletinError(w, http.StatusBadRequest, "cannot_renew", "Event notices end with their event.")
			return
		}
		row.ExpiresAt = time.Now().Add(bulletinPostLifetime)
		if row.Status != structs.BulletinStatusActive {
			row.Status = structs.BulletinStatusActive
		}
	}

	if err := a.db.UpdateBulletinPost(r.Context(), row); err != nil {
		if errors.Is(err, db.ErrBulletinNotFound) {
			bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
			return
		}
		a.logger.Logf("error updating bulletin post %d: %s", row.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updated, err := a.db.GetBulletinPost(r.Context(), row.Id, userId)
	if err != nil {
		a.logger.Logf("error reloading bulletin post %d: %s", row.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	posts, err := a.buildBulletinPosts(r.Context(), userId, []*structs.BulletinPostRow{updated}, false)
	if err != nil || len(posts) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, posts[0])
}

func (a *AppService) DeleteBulletinPost(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	row, ok := a.loadOwnBulletinPost(w, r, userId)
	if !ok {
		return
	}
	if err := a.db.RemoveBulletinPost(r.Context(), row.Id, userId, "author"); err != nil && !errors.Is(err, db.ErrBulletinNotFound) {
		a.logger.Logf("error removing bulletin post %d: %s", row.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Photos -------------------------------------------------------------------------

func (a *AppService) UploadBulletinPhoto(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	row, ok := a.loadOwnBulletinPost(w, r, userId)
	if !ok {
		return
	}

	count, err := a.db.CountBulletinPostPhotos(r.Context(), row.Id)
	if err != nil {
		a.logger.Logf("error counting photos for post %d: %s", row.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if count >= bulletinMaxPhotos {
		bulletinError(w, http.StatusBadRequest, "too_many_photos", fmt.Sprintf("Posts can have up to %d photos.", bulletinMaxPhotos))
		return
	}

	if err := r.ParseMultipartForm(bulletinMaxPhotoBytes + (1 << 20)); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_upload", "Could not read the upload.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("photo")
	if err != nil {
		bulletinError(w, http.StatusBadRequest, "photo_required", "A photo file is required.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, bulletinMaxPhotoBytes+1))
	if err != nil || len(data) == 0 {
		bulletinError(w, http.StatusBadRequest, "empty_photo", "That photo file was empty.")
		return
	}
	if len(data) > bulletinMaxPhotoBytes {
		bulletinError(w, http.StatusRequestEntityTooLarge, "photo_too_large", "That photo is too large. Try a smaller one.")
		return
	}
	// Raster images only: an SVG is an active document and these are served to
	// other users.
	contentType := locationIconContentType(data)
	if contentType == "" {
		bulletinError(w, http.StatusBadRequest, "bad_photo_type", "Photos must be a PNG, JPEG, or WebP image.")
		return
	}

	photo, err := a.db.AddBulletinPostPhoto(r.Context(), row.Id, contentType, data)
	if err != nil {
		a.logger.Logf("error storing photo for post %d: %s", row.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, photo)
}

func (a *AppService) DeleteBulletinPhoto(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	row, ok := a.loadOwnBulletinPost(w, r, userId)
	if !ok {
		return
	}
	photoId, err := strconv.ParseInt(chi.URLParam(r, "photoId"), 10, 64)
	if err != nil {
		bulletinError(w, http.StatusNotFound, "not_found", "That photo could not be found.")
		return
	}
	if err := a.db.DeleteBulletinPostPhoto(r.Context(), row.Id, photoId); err != nil {
		if errors.Is(err, db.ErrBulletinNotFound) {
			bulletinError(w, http.StatusNotFound, "not_found", "That photo could not be found.")
			return
		}
		a.logger.Logf("error deleting photo %d: %s", photoId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetBulletinPhoto serves a photo to anyone who may see its post (and to
// admins, who review reported posts).
func (a *AppService) GetBulletinPhoto(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	photoId, err := strconv.ParseInt(chi.URLParam(r, "photoId"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	postId, contentType, data, err := a.db.GetBulletinPhotoData(r.Context(), photoId)
	if errors.Is(err, db.ErrBulletinNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error loading photo %d: %s", photoId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := a.db.GetBulletinPost(r.Context(), postId, userId); err != nil {
		if !a.IsAdmin(r.Context(), userId) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// --- Trade feedback --------------------------------------------------------------------

func (a *AppService) PostBulletinFeedback(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	postId, ok := bulletinPostIdFromPath(r)
	if !ok {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return
	}
	var req structs.BulletinFeedbackRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil || req.ToUserId == "" || req.ToUserId == userId {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the feedback.")
		return
	}

	post, err := a.db.GetBulletinPostAny(r.Context(), postId)
	if err != nil {
		bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
		return
	}
	// Feedback is between the post's author and someone who messaged them
	// about it; nobody else can rate either side.
	if post.AuthorId != userId && post.AuthorId != req.ToUserId {
		bulletinError(w, http.StatusConflict, "not_a_trade", "You can only leave feedback on your own trades.")
		return
	}
	traded, err := a.db.UsersTradedOnPost(r.Context(), postId, userId, req.ToUserId)
	if err != nil {
		a.logger.Logf("error checking trade on post %d: %s", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !traded {
		bulletinError(w, http.StatusConflict, "not_a_trade", "You can only leave feedback on your own trades.")
		return
	}
	if err := a.db.UpsertBulletinTradeFeedback(r.Context(), postId, userId, req.ToUserId, req.Positive); err != nil {
		a.logger.Logf("error saving feedback on post %d: %s", postId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Search --------------------------------------------------------------------------------

func (a *AppService) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(strings.TrimPrefix(q, "@")) < 2 {
		writeJSON(w, http.StatusOK, structs.UserSearchResult{Users: []structs.PublicUser{}, Merchants: []structs.UserSearchMerchant{}})
		return
	}
	if utf8.RuneCountInString(q) > 40 {
		q = truncateRunes(q, 40)
	}

	ctx := r.Context()
	userIds, merchants, err := a.db.SearchSFLuvUsers(ctx, userId, q)
	if err != nil {
		a.logger.Logf("error searching users for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Your own address book, searched alongside the public directory.
	//
	// This is what makes "I saved Maria's QR code, now I want to message her"
	// work. The public search above only finds people who opted into being
	// findable, which is the right rule for strangers and the wrong one for
	// somebody whose wallet address you already have. See
	// SearchUsersViaContacts for why that is not a privacy regression.
	//
	// Contact matches come FIRST: somebody typing a name they chose is looking
	// for the person they chose it for, not for a stranger who happens to share
	// it.
	contactNames, err := a.db.SearchUsersViaContacts(ctx, userId, q)
	if err != nil {
		// Not fatal. The public results are still worth returning, and a
		// failure here is one missing row rather than a broken search.
		a.logger.Logf("error searching contacts for %s: %s", userId, err)
		contactNames = map[string]string{}
	}

	ordered := make([]string, 0, len(userIds)+len(contactNames))
	seen := map[string]struct{}{}
	for id := range contactNames {
		ordered = append(ordered, id)
		seen[id] = struct{}{}
	}
	for _, id := range userIds {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	people, err := a.db.GetPublicUsers(ctx, ordered)
	if err != nil {
		a.logger.Logf("error resolving search results: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result := structs.UserSearchResult{Users: []structs.PublicUser{}, Merchants: merchants}
	for _, id := range ordered {
		person, found := people[id]
		if !found {
			continue
		}
		// A contact match is labelled with the name the searcher saved, not the
		// public name the account chose — which they may never have seen, and
		// which would make the result look like somebody else.
		if saved, isContact := contactNames[id]; isContact && strings.TrimSpace(saved) != "" {
			person.PublicName = saved
			person.SavedContact = true
		}
		result.Users = append(result.Users, person)
	}
	writeJSON(w, http.StatusOK, result)
}

// GetPublicUserProfile shows one person's public face (name, badges, good
// trades), for the seller row and chat headers.
func (a *AppService) GetPublicUserProfile(w http.ResponseWriter, r *http.Request) {
	viewer, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	targetId := chi.URLParam(r, "userId")
	blocked, err := a.db.UsersAreBlocked(r.Context(), viewer, targetId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if blocked && targetId != viewer {
		bulletinError(w, http.StatusNotFound, "not_found", "That person could not be found.")
		return
	}
	people, err := a.db.GetPublicUsers(r.Context(), []string{targetId})
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p, ok := people[targetId]
	if !ok {
		bulletinError(w, http.StatusNotFound, "not_found", "That person could not be found.")
		return
	}
	writeJSON(w, http.StatusOK, p)
}
