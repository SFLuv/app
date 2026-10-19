package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrBulletinNotFound is returned when a post does not exist or the caller may
// not act on it. One error for both, so a request cannot probe for posts it is
// not allowed to see.
var ErrBulletinNotFound = errors.New("bulletin post not found")

// ErrHandleTaken is returned when another account already holds a handle.
var ErrHandleTaken = errors.New("handle already taken")

const bulletinEarthRadiusMiles = 3958.8

// --- Profiles ---------------------------------------------------------------

func (a *AppDB) GetUserProfile(ctx context.Context, userId string) (*structs.UserProfile, error) {
	row := a.db.QueryRow(ctx, `
		SELECT user_id, public_name, handle, discoverable, bulletin_suspended_until
		FROM user_profiles
		WHERE user_id = $1;
	`, userId)

	var p structs.UserProfile
	if err := row.Scan(&p.UserId, &p.PublicName, &p.Handle, &p.Discoverable, &p.SuspendedUntil); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Exists = true
	return &p, nil
}

func (a *AppDB) GetUserContactName(ctx context.Context, userId string) (string, error) {
	var name string
	err := a.db.QueryRow(ctx, `SELECT COALESCE(contact_name, '') FROM users WHERE id = $1;`, userId).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

func (a *AppDB) UpsertUserProfile(ctx context.Context, userId string, req *structs.UserProfileRequest) (*structs.UserProfile, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO user_profiles(user_id, public_name, handle, discoverable)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			public_name = EXCLUDED.public_name,
			handle = EXCLUDED.handle,
			discoverable = EXCLUDED.discoverable,
			updated_at = NOW();
	`, userId, req.PublicName, req.Handle, req.Discoverable)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrHandleTaken
		}
		return nil, err
	}
	return a.GetUserProfile(ctx, userId)
}

// BulletinSuspendedUntil returns when a user's Bulletin suspension ends, or nil
// when they are not suspended.
func (a *AppDB) BulletinSuspendedUntil(ctx context.Context, userId string) (*time.Time, error) {
	var until *time.Time
	err := a.db.QueryRow(ctx, `
		SELECT bulletin_suspended_until
		FROM user_profiles
		WHERE user_id = $1 AND bulletin_suspended_until > NOW();
	`, userId).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return until, err
}

// GetPublicUsers resolves the public face of a set of users in one query:
// name, handle, badges, and how many trades others marked as good.
func (a *AppDB) GetPublicUsers(ctx context.Context, userIds []string) (map[string]structs.PublicUser, error) {
	out := map[string]structs.PublicUser{}
	if len(userIds) == 0 {
		return out, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT
			u.id,
			COALESCE(up.public_name, ''),
			up.handle,
			(u.is_merchant OR u.account_type = 'merchant'),
			u.is_improver,
			EXISTS (
				SELECT 1 FROM user_credentials c
				WHERE c.user_id = u.id
					AND c.credential_type = 'sfluv_certified_volunteer'
					AND c.is_revoked = FALSE
			),
			(
				SELECT COUNT(*) FROM bulletin_trade_feedback f
				WHERE f.to_user = u.id AND f.positive = TRUE
			)
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = ANY($1);
	`, userIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			pu                               structs.PublicUser
			isMerchant, isImprover, verified bool
		)
		if err := rows.Scan(&pu.UserId, &pu.PublicName, &pu.Handle, &isMerchant, &isImprover, &verified, &pu.GoodTrades); err != nil {
			return nil, err
		}
		if pu.PublicName == "" {
			pu.PublicName = "SFLuv neighbor"
		}
		pu.Badges = []string{}
		if isMerchant {
			pu.Badges = append(pu.Badges, structs.BulletinBadgeMerchant)
		}
		if verified {
			pu.Badges = append(pu.Badges, structs.BulletinBadgeVerifiedVolunteer)
		}
		if isImprover {
			pu.Badges = append(pu.Badges, structs.BulletinBadgeImprover)
		}
		out[pu.UserId] = pu
	}
	return out, rows.Err()
}

// UsersAreBlocked reports whether either user has blocked the other.
func (a *AppDB) UsersAreBlocked(ctx context.Context, userA, userB string) (bool, error) {
	var blocked bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_blocks
			WHERE (blocker_id = $1 AND blocked_id = $2)
				OR (blocker_id = $2 AND blocked_id = $1)
		);
	`, userA, userB).Scan(&blocked)
	return blocked, err
}

// --- Posts -------------------------------------------------------------------

const bulletinPostColumns = `
	p.id,
	p.author_id,
	p.category,
	p.title,
	p.body,
	p.price_sfluv::text,
	p.accepts_sfluv,
	p.accepts_cash,
	p.approx_lat::float8,
	p.approx_lng::float8,
	p.area_label,
	p.meetup_location_id,
	p.linked_event_id,
	p.status,
	p.expires_at,
	p.created_at`

func scanBulletinPostRow(row pgx.Row, p *structs.BulletinPostRow, extra ...any) error {
	dest := []any{
		&p.Id, &p.AuthorId, &p.Category, &p.Title, &p.Body, &p.PriceSfluv,
		&p.AcceptsSfluv, &p.AcceptsCash, &p.ApproxLat, &p.ApproxLng, &p.AreaLabel,
		&p.MeetupLocationId, &p.LinkedEventId, &p.Status, &p.ExpiresAt, &p.CreatedAt,
	}
	dest = append(dest, extra...)
	return row.Scan(dest...)
}

type NewBulletinPost struct {
	AuthorId         string
	Category         string
	Title            string
	Body             string
	PriceSfluv       *string
	AcceptsSfluv     bool
	AcceptsCash      bool
	ApproxLat        float64
	ApproxLng        float64
	AreaLabel        string
	MeetupLocationId *int
	LinkedEventId    *string
	ExpiresAt        time.Time
}

func (a *AppDB) CreateBulletinPost(ctx context.Context, p *NewBulletinPost) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO bulletin_posts(
			author_id, category, title, body, price_sfluv, accepts_sfluv, accepts_cash,
			approx_lat, approx_lng, area_label, meetup_location_id, linked_event_id, expires_at
		) VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id;
	`, p.AuthorId, p.Category, p.Title, p.Body, p.PriceSfluv, p.AcceptsSfluv, p.AcceptsCash,
		p.ApproxLat, p.ApproxLng, p.AreaLabel, p.MeetupLocationId, p.LinkedEventId, p.ExpiresAt).Scan(&id)
	return id, err
}

// CountRecentBulletinPosts is the rate-limit input: posts a user created since
// the given time, removed ones included so deleting cannot reset the count.
func (a *AppDB) CountRecentBulletinPosts(ctx context.Context, userId string, since time.Time) (int, error) {
	var n int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM bulletin_posts WHERE author_id = $1 AND created_at >= $2;
	`, userId, since).Scan(&n)
	return n, err
}

// GetBulletinPost loads one post for a viewer. Removed posts, posts from or to
// blocked users, and posts by deactivated accounts read as not found. Hidden
// posts stay visible to their author (so they can see what happened) and to
// admins via the moderation path, which does not use this function.
func (a *AppDB) GetBulletinPost(ctx context.Context, postId int64, viewerId string) (*structs.BulletinPostRow, error) {
	row := a.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM bulletin_posts p
		JOIN users u ON u.id = p.author_id AND u.active = TRUE
		WHERE p.id = $1
			AND p.status <> 'removed'
			AND (p.status <> 'hidden' OR p.author_id = $2)
			AND NOT EXISTS (
				SELECT 1 FROM user_blocks b
				WHERE (b.blocker_id = $2 AND b.blocked_id = p.author_id)
					OR (b.blocker_id = p.author_id AND b.blocked_id = $2)
			);
	`, bulletinPostColumns), postId, viewerId)

	var p structs.BulletinPostRow
	if err := scanBulletinPostRow(row, &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBulletinNotFound
		}
		return nil, err
	}
	return &p, nil
}

// GetBulletinPostAny loads a post regardless of status or blocks. For admin
// review and for internal checks only; never return its result to a user
// without re-checking visibility.
func (a *AppDB) GetBulletinPostAny(ctx context.Context, postId int64) (*structs.BulletinPostRow, error) {
	row := a.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM bulletin_posts p WHERE p.id = $1;
	`, bulletinPostColumns), postId)
	var p structs.BulletinPostRow
	if err := scanBulletinPostRow(row, &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBulletinNotFound
		}
		return nil, err
	}
	return &p, nil
}

// GetBulletinFeed returns active, unexpired posts within the radius, nearest
// first. The bounding box lets the partial index do the narrowing; the exact
// great-circle distance then orders and trims. Distance ties break on id, which
// is also the cursor, so paging is stable.
func (a *AppDB) GetBulletinFeed(ctx context.Context, f *structs.BulletinFeedRequest) ([]*structs.BulletinPostRow, bool, error) {
	latDelta := f.RadiusMiles / 69.0
	cosLat := math.Cos(f.Lat * math.Pi / 180)
	if cosLat < 0.01 {
		cosLat = 0.01
	}
	lngDelta := f.RadiusMiles / (69.0 * cosLat)

	args := []any{
		f.ViewerId,
		f.Lat, f.Lng,
		f.Lat - latDelta, f.Lat + latDelta,
		f.Lng - lngDelta, f.Lng + lngDelta,
		f.Category,
		f.RadiusMiles,
		f.Limit + 1,
	}
	cursorClause := ""
	if f.AfterDistance != nil && f.AfterId != nil {
		args = append(args, *f.AfterDistance, *f.AfterId)
		cursorClause = fmt.Sprintf("AND (q.distance_miles, q.id) > ($%d, $%d)", len(args)-1, len(args))
	}

	query := fmt.Sprintf(`
		SELECT q.* FROM (
			SELECT %s,
				%f * 2 * ASIN(SQRT(
					POWER(SIN(RADIANS(p.approx_lat::float8 - $2) / 2), 2)
					+ COS(RADIANS($2)) * COS(RADIANS(p.approx_lat::float8))
					* POWER(SIN(RADIANS(p.approx_lng::float8 - $3) / 2), 2)
				)) AS distance_miles
			FROM bulletin_posts p
			JOIN users u ON u.id = p.author_id AND u.active = TRUE
			LEFT JOIN user_profiles up ON up.user_id = p.author_id
			WHERE p.status = 'active'
				AND p.expires_at > NOW()
				AND p.approx_lat BETWEEN $4 AND $5
				AND p.approx_lng BETWEEN $6 AND $7
				AND ($8 = '' OR p.category = $8)
				AND (up.bulletin_suspended_until IS NULL OR up.bulletin_suspended_until <= NOW())
				AND NOT EXISTS (
					SELECT 1 FROM user_blocks b
					WHERE (b.blocker_id = $1 AND b.blocked_id = p.author_id)
						OR (b.blocker_id = p.author_id AND b.blocked_id = $1)
				)
				AND NOT EXISTS (
					SELECT 1 FROM content_reports r
					WHERE r.reporter_id = $1 AND r.target_type = 'post' AND r.target_id = p.id::text
				)
		) q
		WHERE q.distance_miles <= $9
			%s
		ORDER BY q.distance_miles, q.id
		LIMIT $10;
	`, bulletinPostColumns, bulletinEarthRadiusMiles, cursorClause)

	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	posts := []*structs.BulletinPostRow{}
	for rows.Next() {
		var p structs.BulletinPostRow
		var distance float64
		if err := scanBulletinPostRow(rows, &p, &distance); err != nil {
			return nil, false, err
		}
		p.DistanceMiles = &distance
		posts = append(posts, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(posts) > f.Limit
	if hasMore {
		posts = posts[:f.Limit]
	}
	return posts, hasMore, nil
}

// GetMyBulletinPosts lists a user's own posts, newest first, whatever their
// status (except removed by the author).
func (a *AppDB) GetMyBulletinPosts(ctx context.Context, userId string) ([]*structs.BulletinPostRow, error) {
	rows, err := a.db.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM bulletin_posts p
		WHERE p.author_id = $1 AND p.status <> 'removed'
		ORDER BY p.created_at DESC
		LIMIT 100;
	`, bulletinPostColumns), userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []*structs.BulletinPostRow{}
	for rows.Next() {
		var p structs.BulletinPostRow
		if err := scanBulletinPostRow(rows, &p); err != nil {
			return nil, err
		}
		posts = append(posts, &p)
	}
	return posts, rows.Err()
}

// UpdateBulletinPost applies an author's edit. The caller has already checked
// ownership and validated the merged result; this writes it.
func (a *AppDB) UpdateBulletinPost(ctx context.Context, p *structs.BulletinPostRow) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE bulletin_posts SET
			title = $3,
			body = $4,
			price_sfluv = $5::numeric,
			accepts_sfluv = $6,
			accepts_cash = $7,
			meetup_location_id = $8,
			status = $9,
			expires_at = $10,
			updated_at = NOW()
		WHERE id = $1 AND author_id = $2 AND status <> 'removed';
	`, p.Id, p.AuthorId, p.Title, p.Body, p.PriceSfluv, p.AcceptsSfluv, p.AcceptsCash,
		p.MeetupLocationId, p.Status, p.ExpiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBulletinNotFound
	}
	return nil
}

func (a *AppDB) RemoveBulletinPost(ctx context.Context, postId int64, authorId string, reason string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE bulletin_posts SET status = 'removed', removed_reason = $3, updated_at = NOW()
		WHERE id = $1 AND ($2 = '' OR author_id = $2) AND status <> 'removed';
	`, postId, authorId, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBulletinNotFound
	}
	return nil
}

// SetBulletinPostStatus is used by moderation (hide on report threshold,
// restore on dismiss). It does not check ownership.
func (a *AppDB) SetBulletinPostStatus(ctx context.Context, postId int64, fromStatus, toStatus string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE bulletin_posts SET status = $3, updated_at = NOW()
		WHERE id = $1 AND status = $2;
	`, postId, fromStatus, toStatus)
	return err
}

// --- Photos ------------------------------------------------------------------

func (a *AppDB) AddBulletinPostPhoto(ctx context.Context, postId int64, contentType string, data []byte) (*structs.BulletinPhoto, error) {
	var photo structs.BulletinPhoto
	err := a.db.QueryRow(ctx, `
		INSERT INTO bulletin_post_photos(post_id, position, content_type, image_data, size_bytes)
		SELECT $1, COALESCE(MAX(position) + 1, 0), $2, $3, $4
		FROM bulletin_post_photos WHERE post_id = $1
		RETURNING id, position;
	`, postId, contentType, data, len(data)).Scan(&photo.Id, &photo.Position)
	return &photo, err
}

func (a *AppDB) CountBulletinPostPhotos(ctx context.Context, postId int64) (int, error) {
	var n int
	err := a.db.QueryRow(ctx, `SELECT COUNT(*) FROM bulletin_post_photos WHERE post_id = $1;`, postId).Scan(&n)
	return n, err
}

func (a *AppDB) DeleteBulletinPostPhoto(ctx context.Context, postId, photoId int64) error {
	tag, err := a.db.Exec(ctx, `DELETE FROM bulletin_post_photos WHERE id = $1 AND post_id = $2;`, photoId, postId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBulletinNotFound
	}
	return nil
}

func (a *AppDB) GetBulletinPhotosForPosts(ctx context.Context, postIds []int64) (map[int64][]structs.BulletinPhoto, error) {
	out := map[int64][]structs.BulletinPhoto{}
	if len(postIds) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT post_id, id, position FROM bulletin_post_photos
		WHERE post_id = ANY($1)
		ORDER BY post_id, position;
	`, postIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var postId int64
		var photo structs.BulletinPhoto
		if err := rows.Scan(&postId, &photo.Id, &photo.Position); err != nil {
			return nil, err
		}
		out[postId] = append(out[postId], photo)
	}
	return out, rows.Err()
}

// GetBulletinPhotoData returns a photo's bytes along with the post it belongs
// to, so the handler can check the viewer may see that post.
func (a *AppDB) GetBulletinPhotoData(ctx context.Context, photoId int64) (int64, string, []byte, error) {
	var (
		postId      int64
		contentType string
		data        []byte
	)
	err := a.db.QueryRow(ctx, `
		SELECT post_id, content_type, image_data FROM bulletin_post_photos WHERE id = $1;
	`, photoId).Scan(&postId, &contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil, ErrBulletinNotFound
	}
	return postId, contentType, data, err
}

// --- Meetup locations ---------------------------------------------------------

// GetApprovedLocationNames resolves meetup spots. Only approved merchant
// locations can be a meetup spot.
func (a *AppDB) GetApprovedLocationNames(ctx context.Context, ids []int) (map[int]string, error) {
	out := map[int]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT id, name FROM locations WHERE id = ANY($1) AND approval = TRUE;
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// --- Trade feedback -------------------------------------------------------------

// UsersTradedOnPost reports whether the two users have a conversation about the
// post, which is the minimum evidence a trade happened before feedback counts.
func (a *AppDB) UsersTradedOnPost(ctx context.Context, postId int64, userA, userB string) (bool, error) {
	low, high := orderedPair(userA, userB)
	var ok bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM conversations
			WHERE user_low = $1 AND user_high = $2 AND post_id = $3
		);
	`, low, high, postId).Scan(&ok)
	return ok, err
}

func (a *AppDB) UpsertBulletinTradeFeedback(ctx context.Context, postId int64, from, to string, positive bool) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO bulletin_trade_feedback(post_id, from_user, to_user, positive)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (post_id, from_user) DO UPDATE SET
			to_user = EXCLUDED.to_user,
			positive = EXCLUDED.positive,
			created_at = NOW();
	`, postId, from, to, positive)
	return err
}

// --- Search -------------------------------------------------------------------

// SearchSFLuvUsers finds people who chose to be findable (by public name prefix
// or exact handle) and approved merchant locations (by name). An exact handle
// match finds anyone who set that handle, discoverable or not: knowing the
// handle is the permission. Blocked users never appear. No location is returned.
func (a *AppDB) SearchSFLuvUsers(ctx context.Context, viewerId string, query string) ([]string, []structs.UserSearchMerchant, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	handle := strings.TrimPrefix(q, "@")
	pattern := escapeLike(q) + "%"

	rows, err := a.db.Query(ctx, `
		SELECT up.user_id
		FROM user_profiles up
		JOIN users u ON u.id = up.user_id AND u.active = TRUE
		WHERE up.user_id <> $1
			AND (
				(up.discoverable = TRUE AND LOWER(up.public_name) LIKE $2 ESCAPE '\')
				OR (up.handle IS NOT NULL AND up.handle = $3)
			)
			AND NOT EXISTS (
				SELECT 1 FROM user_blocks b
				WHERE (b.blocker_id = $1 AND b.blocked_id = up.user_id)
					OR (b.blocker_id = up.user_id AND b.blocked_id = $1)
			)
		ORDER BY (up.handle = $3) DESC NULLS LAST, LOWER(up.public_name)
		LIMIT 20;
	`, viewerId, pattern, handle)
	if err != nil {
		return nil, nil, err
	}
	userIds := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		userIds = append(userIds, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	merchantRows, err := a.db.Query(ctx, `
		SELECT l.id, l.name, l.owner_id
		FROM locations l
		JOIN users u ON u.id = l.owner_id AND u.active = TRUE
		WHERE l.approval = TRUE
			AND l.owner_id IS NOT NULL
			AND l.owner_id <> $1
			AND LOWER(l.name) LIKE '%' || $2 ESCAPE '\'
		ORDER BY l.name
		LIMIT 10;
	`, viewerId, pattern)
	if err != nil {
		return nil, nil, err
	}
	defer merchantRows.Close()
	merchants := []structs.UserSearchMerchant{}
	for merchantRows.Next() {
		var m structs.UserSearchMerchant
		if err := merchantRows.Scan(&m.LocationId, &m.Name, &m.OwnerUserId); err != nil {
			return nil, nil, err
		}
		merchants = append(merchants, m)
	}
	return userIds, merchants, merchantRows.Err()
}

// --- helpers -------------------------------------------------------------------

func orderedPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// FormatBulletinCursor and ParseBulletinCursor encode the feed position as
// "<distance>:<id>". Opaque to the client.
func FormatBulletinCursor(distance float64, id int64) string {
	return strconv.FormatFloat(distance, 'f', -1, 64) + ":" + strconv.FormatInt(id, 10)
}

func ParseBulletinCursor(cursor string) (*float64, *int64, error) {
	if cursor == "" {
		return nil, nil, nil
	}
	parts := strings.SplitN(cursor, ":", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("malformed cursor")
	}
	d, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, nil, fmt.Errorf("malformed cursor")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("malformed cursor")
	}
	return &d, &id, nil
}
