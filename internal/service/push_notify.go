package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Push notifications for the mobile app (phase 6b): a title on your
// watchlist arriving on one of your streaming services in your country.
//
// The dispatch job follows watch_provider_events (written when a title's
// TMDB watch providers are refreshed) with its own cursor in app_settings,
// the same way webhooks do, so history is never sent: the first run only
// sets the cursor. Each (user, title) is notified once (notification_log).

const (
	notifyKindArrival   = "watchlist_arrival"
	cursorNotifyOffers  = "notifications.cursor.offers"
	notifyEventsPage    = 500
	notifyPagesPerRun   = 10
	notifyReceiptAge    = "-15 minutes"
	arrivalChannel      = "arrivals"
	maxDeviceAppVersion = 32
)

// Monetizations that count as "on your services" (not rent or buy).
var arrivalMonetizations = map[string]bool{"flatrate": true, "free": true, "ads": true}

var expoPushTokenRe = regexp.MustCompile(`^Expo(nent)?PushToken\[[A-Za-z0-9_-]{8,128}\]$`)

type pushSender interface {
	Send(ctx context.Context, msgs []client.PushMessage) ([]client.PushTicket, error)
	Receipts(ctx context.Context, ids []string) (map[string]client.PushReceipt, error)
}

type offerEventReader interface {
	OfferEventsAfter(ctx context.Context, afterID, limit int64) ([]repository.OfferEvent, error)
	LatestOfferEventID(ctx context.Context) (int64, error)
}

type settingsStore interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// PushService registers devices and sends push notifications.
type PushService struct {
	log     *slog.Logger
	repo    *repository.PushRepository
	offers  offerEventReader
	ops     settingsStore
	sender  pushSender
	enabled bool
}

func NewPushService(log *slog.Logger, repo *repository.PushRepository, offers offerEventReader, ops settingsStore, sender pushSender, enabled bool) *PushService {
	return &PushService{log: log, repo: repo, offers: offers, ops: ops, sender: sender, enabled: enabled}
}

// Enabled reports whether the server sends push notifications.
func (s *PushService) Enabled() bool { return s.enabled }

// DeviceInput is what the app sends to register for push.
type DeviceInput struct {
	Platform   string `json:"platform"`
	Token      string `json:"token"`
	AppVersion string `json:"app_version"`
}

// RegisterDevice stores (or refreshes) the device's push token for the user
// and the session that registered it.
func (s *PushService) RegisterDevice(ctx context.Context, userID int64, sessionID *int64, in DeviceInput) (repository.UserDevice, error) {
	platform := strings.ToLower(strings.TrimSpace(in.Platform))
	token := strings.TrimSpace(in.Token)
	version := strings.TrimSpace(in.AppVersion)
	switch {
	case platform != "ios" && platform != "android":
		return repository.UserDevice{}, &ValidationError{Message: "platform must be ios or android"}
	case !expoPushTokenRe.MatchString(token):
		return repository.UserDevice{}, &ValidationError{Message: "token must be an Expo push token"}
	case len(version) > maxDeviceAppVersion:
		return repository.UserDevice{}, &ValidationError{Message: "app_version is too long"}
	}
	return s.repo.UpsertDevice(ctx, userID, sessionID, platform, token, version)
}

func (s *PushService) Devices(ctx context.Context, userID int64) ([]repository.UserDevice, error) {
	return s.repo.ListDevices(ctx, userID)
}

// DeleteDevice unregisters one of the user's devices (sql.ErrNoRows if it
// isn't theirs).
func (s *PushService) DeleteDevice(ctx context.Context, userID, id int64) error {
	return s.repo.DeleteDevice(ctx, userID, id)
}

// PushRunSummary is what one dispatch run did.
type PushRunSummary struct {
	Skipped         string `json:"skipped,omitempty"`
	Events          int    `json:"events"`
	Notifications   int    `json:"notifications"`
	Messages        int    `json:"messages"`
	Failed          int    `json:"failed"`
	DevicesDisabled int    `json:"devices_disabled"`
	ReceiptsChecked int    `json:"receipts_checked"`
	Cursor          int64  `json:"cursor"`
}

// arrival is a title becoming available on a service in a country.
type arrival struct {
	country      string
	providerID   int64
	providerName string
}

type pendingPush struct {
	userID   int64
	ref      string
	deviceID int64
	msg      client.PushMessage
}

// Run checks receipts of earlier sends, then turns new provider events into
// watchlist-arrival notifications.
func (s *PushService) Run(ctx context.Context, runLog func(level, msg string)) (*PushRunSummary, error) {
	sum := &PushRunSummary{}
	if !s.enabled {
		sum.Skipped = "push notifications are off (PUSH_ENABLED)"
		runLog("info", sum.Skipped)
		return sum, nil
	}
	if err := s.checkReceipts(ctx, sum); err != nil {
		// Receipts are housekeeping; sending goes ahead.
		runLog("warn", "checking push receipts failed: "+err.Error())
	}
	cursor, err := s.ops.Setting(ctx, cursorNotifyOffers)
	if err != nil {
		return nil, err
	}
	if cursor == "" {
		latest, err := s.offers.LatestOfferEventID(ctx)
		if err != nil {
			return nil, err
		}
		sum.Cursor = latest
		runLog("info", fmt.Sprintf("started following watch provider changes after event %d; earlier ones are not sent", latest))
		return sum, s.ops.SetSetting(ctx, cursorNotifyOffers, strconv.FormatInt(latest, 10))
	}
	after, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("bad notification cursor %q: %w", cursor, err)
	}
	for range notifyPagesPerRun {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		events, err := s.offers.OfferEventsAfter(ctx, after, notifyEventsPage)
		if err != nil {
			return sum, err
		}
		if len(events) == 0 {
			break
		}
		if err := s.dispatch(ctx, events, sum); err != nil {
			return sum, err
		}
		after = events[len(events)-1].ID
		if err := s.ops.SetSetting(ctx, cursorNotifyOffers, strconv.FormatInt(after, 10)); err != nil {
			return sum, err
		}
		sum.Events += len(events)
		if len(events) < notifyEventsPage {
			break
		}
	}
	sum.Cursor = after
	runLog("info", fmt.Sprintf("%d provider changes read, %d notifications (%d messages, %d failed), %d devices disabled",
		sum.Events, sum.Notifications, sum.Messages, sum.Failed, sum.DevicesDisabled))
	return sum, nil
}

func (s *PushService) dispatch(ctx context.Context, events []repository.OfferEvent, sum *PushRunSummary) error {
	type titleKey struct{ kind, id string }
	byTitle := map[titleKey][]arrival{}
	names := map[titleKey]string{}
	var order []titleKey
	for _, e := range events {
		if e.Event != "added" || !arrivalMonetizations[e.Monetization] {
			continue
		}
		k := titleKey{e.TmdbKind, e.TmdbID}
		if _, ok := byTitle[k]; !ok {
			order = append(order, k)
		}
		byTitle[k] = append(byTitle[k], arrival{country: strings.ToUpper(e.Country), providerID: e.ProviderID, providerName: e.ProviderName})
		if e.Title != "" {
			names[k] = e.Title
		}
	}
	var pending []pendingPush
	for _, k := range order {
		watchers, err := s.repo.ArrivalWatchers(ctx, k.kind, k.id)
		if err != nil {
			return err
		}
		for _, w := range watchers {
			a, ok := matchArrival(w.Preferences, byTitle[k])
			if !ok {
				continue
			}
			ref := k.kind + ":" + k.id
			fresh, err := s.repo.LogNotification(ctx, w.UserID, notifyKindArrival, ref)
			if err != nil {
				return err
			}
			if !fresh {
				continue
			}
			tokens, err := s.repo.ActiveTokens(ctx, w.UserID)
			if err != nil {
				return err
			}
			sum.Notifications++
			msg := arrivalMessage(names[k], a, k.kind, k.id)
			for _, t := range tokens {
				m := msg
				m.To = t.Token
				pending = append(pending, pendingPush{userID: w.UserID, ref: ref, deviceID: t.ID, msg: m})
			}
		}
	}
	return s.send(ctx, pending, sum)
}

// matchArrival picks the arrival the user cares about: opted in, in their
// country, on one of their services (any service when they haven't listed
// any).
func matchArrival(prefsJSON string, arrivals []arrival) (arrival, bool) {
	var p Preferences
	if err := json.Unmarshal([]byte(prefsJSON), &p); err != nil {
		return arrival{}, false
	}
	if p.NotifyWatchlistArrivals == nil || !*p.NotifyWatchlistArrivals || p.Country == nil {
		return arrival{}, false
	}
	country := strings.ToUpper(*p.Country)
	for _, a := range arrivals {
		if a.country == country && (len(p.Services) == 0 || slices.Contains(p.Services, a.providerID)) {
			return a, true
		}
	}
	return arrival{}, false
}

func arrivalMessage(title string, a arrival, kind, id string) client.PushMessage {
	if title == "" {
		title = "A title on your watchlist"
	}
	return client.PushMessage{
		Title:     "Now on " + a.providerName,
		Body:      title + " from your watchlist is streaming on " + a.providerName + ".",
		Data:      map[string]any{"url": "/title/" + kind + "/" + id},
		Sound:     "default",
		ChannelID: arrivalChannel,
	}
}

func (s *PushService) send(ctx context.Context, pending []pendingPush, sum *PushRunSummary) error {
	for start := 0; start < len(pending); start += client.ExpoPushMaxBatch {
		batch := pending[start:min(start+client.ExpoPushMaxBatch, len(pending))]
		msgs := make([]client.PushMessage, len(batch))
		for i, p := range batch {
			msgs[i] = p.msg
		}
		tickets, err := s.sender.Send(ctx, msgs)
		if err != nil {
			// Nothing in this batch was accepted: forget those notifications
			// so the next run sends them again (the cursor has not moved past
			// this page yet), then stop.
			for _, p := range pending[start:] {
				if ferr := s.repo.ForgetNotification(ctx, p.userID, notifyKindArrival, p.ref); ferr != nil {
					s.log.Warn("forget notification", "user_id", p.userID, "error", ferr)
				}
			}
			return fmt.Errorf("send push notifications: %w", err)
		}
		sum.Messages += len(batch)
		for i, t := range tickets {
			p := batch[i]
			switch {
			case t.OK:
				if err := s.repo.SaveTicket(ctx, t.ID, p.deviceID); err != nil {
					s.log.Warn("save push ticket", "device_id", p.deviceID, "error", err)
				}
			case t.Error == "DeviceNotRegistered":
				sum.Failed++
				if err := s.repo.DisableDevice(ctx, p.deviceID); err == nil {
					sum.DevicesDisabled++
				}
			default:
				sum.Failed++
				s.log.Warn("push message rejected", "device_id", p.deviceID, "error", t.Error)
			}
		}
	}
	return nil
}

func (s *PushService) checkReceipts(ctx context.Context, sum *PushRunSummary) error {
	if _, err := s.repo.DeleteStaleTickets(ctx); err != nil {
		return err
	}
	due, err := s.repo.DueTickets(ctx, notifyReceiptAge, client.ExpoPushMaxReceipts)
	if err != nil || len(due) == 0 {
		return err
	}
	ids := make([]string, len(due))
	device := make(map[string]int64, len(due))
	for i, t := range due {
		ids[i] = t.TicketID
		device[t.TicketID] = t.DeviceID
	}
	receipts, err := s.sender.Receipts(ctx, ids)
	if err != nil {
		return err
	}
	var done []string
	for id, r := range receipts {
		done = append(done, id)
		if !r.OK && r.Error == "DeviceNotRegistered" {
			if err := s.repo.DisableDevice(ctx, device[id]); err == nil {
				sum.DevicesDisabled++
			}
		} else if !r.OK {
			s.log.Warn("push delivery failed", "device_id", device[id], "error", r.Error)
		}
	}
	sum.ReceiptsChecked = len(done)
	return s.repo.DeleteTickets(ctx, done)
}

// IsDeviceNotFound reports whether err means the device doesn't exist.
func IsDeviceNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
