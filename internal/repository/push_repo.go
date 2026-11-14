package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// PushRepository stores the devices registered for push notifications, the
// notifications already sent and the push tickets awaiting a receipt.
type PushRepository struct {
	q *sqlc.Queries
}

func NewPushRepository(db *sql.DB) *PushRepository {
	return &PushRepository{q: sqlc.New(db)}
}

// UserDevice is a registered phone as shown to its user (never the token).
type UserDevice struct {
	ID         int64  `json:"id"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	// Disabled: the push service said the token no longer works.
	Disabled  bool   `json:"disabled"`
	SessionID *int64 `json:"session_id,omitempty"`
}

// DeviceToken is a device's push token, for sending only.
type DeviceToken struct {
	ID    int64
	Token string
}

// ArrivalWatcher is a user with a title on their watchlist and a device.
type ArrivalWatcher struct {
	UserID      int64
	Preferences string // JSON
}

// PushTicket is a sent message's ticket, checked for its receipt later.
type PushTicket struct {
	TicketID string
	DeviceID int64
}

func (r *PushRepository) UpsertDevice(ctx context.Context, userID int64, sessionID *int64, platform, token, appVersion string) (UserDevice, error) {
	sid := sql.NullInt64{}
	if sessionID != nil {
		sid = sql.NullInt64{Int64: *sessionID, Valid: true}
	}
	id, err := r.q.UpsertUserDevice(ctx, sqlc.UpsertUserDeviceParams{UserID: userID, SessionID: sid, Platform: platform,
		Token: token, AppVersion: sql.NullString{String: appVersion, Valid: appVersion != ""}})
	if err != nil {
		// The error text never includes the token (sqlc errors carry no values).
		return UserDevice{}, fmt.Errorf("upsert device: %w", err)
	}
	devices, err := r.ListDevices(ctx, userID)
	if err != nil {
		return UserDevice{}, err
	}
	for _, d := range devices {
		if d.ID == id {
			return d, nil
		}
	}
	return UserDevice{}, sql.ErrNoRows
}

func (r *PushRepository) ListDevices(ctx context.Context, userID int64) ([]UserDevice, error) {
	rows, err := r.q.ListUserDevices(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	out := make([]UserDevice, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserDevice{ID: row.ID, Platform: row.Platform, AppVersion: row.AppVersion, CreatedAt: row.CreatedAt,
			LastSeenAt: row.LastSeenAt, Disabled: row.DisabledAt != "", SessionID: nullInt64Ptr(row.SessionID)})
	}
	return out, nil
}

// DeleteDevice removes one of the user's devices; sql.ErrNoRows when it
// isn't theirs or doesn't exist.
func (r *PushRepository) DeleteDevice(ctx context.Context, userID, id int64) error {
	n, err := r.q.DeleteUserDevice(ctx, sqlc.DeleteUserDeviceParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PushRepository) ActiveTokens(ctx context.Context, userID int64) ([]DeviceToken, error) {
	rows, err := r.q.ListActiveDeviceTokens(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list device tokens: %w", err)
	}
	out := make([]DeviceToken, 0, len(rows))
	for _, row := range rows {
		out = append(out, DeviceToken{ID: row.ID, Token: row.Token})
	}
	return out, nil
}

func (r *PushRepository) DisableDevice(ctx context.Context, id int64) error {
	return r.q.DisableUserDevice(ctx, id)
}

func (r *PushRepository) ArrivalWatchers(ctx context.Context, kind, id string) ([]ArrivalWatcher, error) {
	rows, err := r.q.ListArrivalWatchers(ctx, sqlc.ListArrivalWatchersParams{TmdbKind: kind, TmdbID: id})
	if err != nil {
		return nil, fmt.Errorf("list arrival watchers: %w", err)
	}
	out := make([]ArrivalWatcher, 0, len(rows))
	for _, row := range rows {
		out = append(out, ArrivalWatcher{UserID: row.UserID, Preferences: row.Preferences})
	}
	return out, nil
}

// LogNotification records a notification; false when it was already sent.
func (r *PushRepository) LogNotification(ctx context.Context, userID int64, kind, ref string) (bool, error) {
	n, err := r.q.InsertNotificationLog(ctx, sqlc.InsertNotificationLogParams{UserID: userID, Kind: kind, Ref: ref})
	if err != nil {
		return false, fmt.Errorf("log notification: %w", err)
	}
	return n > 0, nil
}

// ForgetNotification undoes LogNotification (nothing could be sent).
func (r *PushRepository) ForgetNotification(ctx context.Context, userID int64, kind, ref string) error {
	return r.q.DeleteNotificationLog(ctx, sqlc.DeleteNotificationLogParams{UserID: userID, Kind: kind, Ref: ref})
}

func (r *PushRepository) SaveTicket(ctx context.Context, ticketID string, deviceID int64) error {
	return r.q.InsertPushTicket(ctx, sqlc.InsertPushTicketParams{TicketID: ticketID, DeviceID: deviceID})
}

// DueTickets lists tickets at least age old (a SQLite modifier such as
// "-15 minutes").
func (r *PushRepository) DueTickets(ctx context.Context, age string, limit int64) ([]PushTicket, error) {
	rows, err := r.q.ListDuePushTickets(ctx, sqlc.ListDuePushTicketsParams{Age: age, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list push tickets: %w", err)
	}
	out := make([]PushTicket, 0, len(rows))
	for _, row := range rows {
		out = append(out, PushTicket{TicketID: row.TicketID, DeviceID: row.DeviceID})
	}
	return out, nil
}

func (r *PushRepository) DeleteTickets(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.q.DeletePushTickets(ctx, ids)
}

func (r *PushRepository) DeleteStaleTickets(ctx context.Context) (int64, error) {
	return r.q.DeleteStalePushTickets(ctx)
}
