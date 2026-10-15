package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// MappingRepository holds the TMDB-keyed mapping shared by FlixPatrol and
// Netflix titles: links between the two sources, sharing a mapping over a
// link, and the admin mapping page.
type MappingRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewMappingRepository(db *sql.DB) *MappingRepository {
	return &MappingRepository{db: db, q: sqlc.New(db)}
}

// TitleLink links a Netflix title and a FlixPatrol title that are the same.
type TitleLink struct {
	NetflixTitleID int64
	TitleID        int64
}

// LinkingNetflixTitle is a Netflix title as link computation needs it.
type LinkingNetflixTitle struct {
	ID        int64
	ShowTitle string
	Kind      string // Netflix's list: movie | tv_show
}

// NetflixLinkWeek is a week (YYYY-MM-DD, a Sunday) a Netflix title charted.
type NetflixLinkWeek struct {
	NetflixTitleID int64
	Week           string
}

// FlixPatrolNetflixWeek is a Netflix week a FlixPatrol title was on a
// Netflix chart of a list (movies | tv_shows).
type FlixPatrolNetflixWeek struct {
	TitleID  int64
	Name     string
	FpName   string
	Category string
	Week     string
}

// FlixPatrolNetflixWeeks returns the Netflix weeks FlixPatrol titles were
// on a Netflix chart, per list.
func (r *MappingRepository) FlixPatrolNetflixWeeks(ctx context.Context) ([]FlixPatrolNetflixWeek, error) {
	fw, err := r.q.ListFlixPatrolNetflixWeeks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list flixpatrol netflix weeks: %w", err)
	}
	out := make([]FlixPatrolNetflixWeek, 0, len(fw))
	for _, w := range fw {
		out = append(out, FlixPatrolNetflixWeek{TitleID: w.ID, Name: w.Name, FpName: w.FpName, Category: w.Category, Week: w.Week})
	}
	return out, nil
}

// NetflixLinkInputs returns every Netflix title and the weeks each charted
// from fromWeek on (global or any imported country).
func (r *MappingRepository) NetflixLinkInputs(ctx context.Context, fromWeek string) ([]LinkingNetflixTitle, []NetflixLinkWeek, error) {
	nt, err := r.q.ListNetflixTitlesForLinking(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix titles for linking: %w", err)
	}
	titles := make([]LinkingNetflixTitle, 0, len(nt))
	for _, t := range nt {
		titles = append(titles, LinkingNetflixTitle{ID: t.ID, ShowTitle: t.ShowTitle, Kind: t.Kind})
	}
	nw, err := r.q.ListNetflixTitleWeeksFrom(ctx, fromWeek)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix title weeks: %w", err)
	}
	weeks := make([]NetflixLinkWeek, 0, len(nw))
	for _, w := range nw {
		weeks = append(weeks, NetflixLinkWeek{NetflixTitleID: w.NetflixTitleID, Week: w.Week})
	}
	return titles, weeks, nil
}

// Links returns every stored link.
func (r *MappingRepository) Links(ctx context.Context) ([]TitleLink, error) {
	rows, err := r.q.ListTitleLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list title links: %w", err)
	}
	out := make([]TitleLink, 0, len(rows))
	for _, l := range rows {
		out = append(out, TitleLink{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID})
	}
	return out, nil
}

// ReplaceLinks adds and removes links in one transaction.
func (r *MappingRepository) ReplaceLinks(ctx context.Context, add, remove []TitleLink) error {
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	for _, l := range remove {
		if err := q.DeleteTitleLink(ctx, sqlc.DeleteTitleLinkParams{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID}); err != nil {
			return fmt.Errorf("delete title link: %w", err)
		}
	}
	for _, l := range add {
		if err := q.InsertTitleLink(ctx, sqlc.InsertTitleLinkParams{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID}); err != nil {
			return fmt.Errorf("insert title link: %w", err)
		}
	}
	return tx.Commit()
}

// LinkedTitles returns the Netflix and FlixPatrol titles that have links.
func (r *MappingRepository) LinkedTitles(ctx context.Context) ([]sqlc.NetflixTitle, []sqlc.Title, error) {
	n, err := r.q.ListLinkedNetflixTitles(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list linked netflix titles: %w", err)
	}
	t, err := r.q.ListLinkedFlixPatrolTitles(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list linked flixpatrol titles: %w", err)
	}
	return n, t, nil
}

// AdoptForNetflix gives a (non-manual) Netflix title a linked FlixPatrol
// title's mapping (tmdbKind is movie | tv). Reports whether it changed a row.
func (r *MappingRepository) AdoptForNetflix(ctx context.Context, id int64, tmdbKind, tmdbID, imdbID string, viaTitleID int64, source string) (bool, error) {
	n, err := r.q.AdoptNetflixTitleMapping(ctx, sqlc.AdoptNetflixTitleMappingParams{
		TmdbKind: nullIfEmpty(tmdbKind), TmdbID: nullIfEmpty(tmdbID), ImdbID: imdbID,
		TitleID: sql.NullInt64{Int64: viaTitleID, Valid: viaTitleID > 0}, MatchSource: nullIfEmpty(source), ID: id,
	})
	if err != nil {
		return false, fmt.Errorf("adopt mapping for netflix title %d: %w", id, err)
	}
	return n > 0, nil
}

// AdoptForFlixPatrol gives a (non-manual) FlixPatrol title a linked Netflix
// title's mapping (kind is movie | tv_show).
func (r *MappingRepository) AdoptForFlixPatrol(ctx context.Context, id int64, kind, tmdbID, imdbID string) (bool, error) {
	n, err := r.q.AdoptFlixPatrolTitleMapping(ctx, sqlc.AdoptFlixPatrolTitleMappingParams{
		Kind: kind, TmdbID: nullIfEmpty(tmdbID), ImdbID: imdbID, ID: id,
	})
	if err != nil {
		return false, fmt.Errorf("adopt mapping for title %d: %w", id, err)
	}
	return n > 0, nil
}

// MappingFilter filters the admin mapping list.
type MappingFilter struct {
	TmdbKind string // "" | movie | tv
	Search   string
	Flag     string // "" | conflict | cross_kind | both | netflix_only | flixpatrol_only | manual
}

// MappingRow is a TMDB title and how many titles of each source map to it.
type MappingRow struct {
	Ref              TmdbRef
	FlixPatrolTitles int64
	NetflixTitles    int64
	Conflicts        int64
	CrossKind        int64
	Manual           int64
	LastSeen         string
	Title            *string
	ReleaseDate      *string
	PosterPath       *string
	ImdbID           *string
}

// ListMappings pages the TMDB titles anything is mapped to. It fetches one
// row more than limit so callers can tell whether there is a next page.
func (r *MappingRepository) ListMappings(ctx context.Context, f MappingFilter, limit, offset int64) ([]MappingRow, error) {
	rows, err := r.q.ListTmdbMappings(ctx, sqlc.ListTmdbMappingsParams{
		TmdbKind: f.TmdbKind, Search: f.Search, Flag: f.Flag, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list tmdb mappings: %w", err)
	}
	out := make([]MappingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, MappingRow{
			Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID.String}, FlixPatrolTitles: row.FlixpatrolTitles, NetflixTitles: row.NetflixTitles,
			Conflicts: row.Conflicts, CrossKind: row.CrossKind, Manual: row.Manual, LastSeen: row.LastSeen,
			Title: nullStringPtr(row.Title), ReleaseDate: nullStringPtr(row.ReleaseDate), PosterPath: nullStringPtr(row.PosterPath), ImdbID: nullStringPtr(row.ImdbID),
		})
	}
	return out, nil
}

// MappedFlixPatrolTitle is a FlixPatrol title and the last date it charted.
type MappedFlixPatrolTitle struct {
	sqlc.Title
	LastRankedOn string
}

// MappedNetflixTitle is a Netflix title and the last week it charted.
type MappedNetflixTitle struct {
	sqlc.NetflixTitle
	LastWeek string
}

// TitlesForTmdbIDs returns the FlixPatrol and Netflix titles mapped to these
// TMDB IDs, of either kind (callers match the kind).
func (r *MappingRepository) TitlesForTmdbIDs(ctx context.Context, ids []string) ([]MappedFlixPatrolTitle, []MappedNetflixTitle, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	nids := make([]sql.NullString, 0, len(ids))
	for _, id := range ids {
		nids = append(nids, sql.NullString{String: id, Valid: true})
	}
	ft, err := r.q.ListFlixPatrolTitlesByTmdbIDs(ctx, nids)
	if err != nil {
		return nil, nil, fmt.Errorf("list flixpatrol titles by tmdb ids: %w", err)
	}
	fp := make([]MappedFlixPatrolTitle, 0, len(ft))
	for _, t := range ft {
		fp = append(fp, MappedFlixPatrolTitle{Title: t.Title, LastRankedOn: t.LastRankedOn})
	}
	nt, err := r.q.ListNetflixTitlesByTmdbIDs(ctx, nids)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix titles by tmdb ids: %w", err)
	}
	nf := make([]MappedNetflixTitle, 0, len(nt))
	for _, t := range nt {
		nf = append(nf, MappedNetflixTitle{NetflixTitle: t.NetflixTitle, LastWeek: t.LastWeek})
	}
	return fp, nf, nil
}

// LinkedFlixPatrol is a FlixPatrol title linked to a Netflix title.
type LinkedFlixPatrol struct {
	NetflixTitleID int64
	TitleID        int64
	Name           string
	Slug           string
	Kind           string // movie | tv_show (TMDB namespace)
	TmdbID         string
	MatchStatus    string
}

// LinkedNetflix is a Netflix title linked to a FlixPatrol title.
type LinkedNetflix struct {
	NetflixTitleID int64
	TitleID        int64
	ShowTitle      string
	Kind           string // Netflix's list: movie | tv_show
	TmdbKind       string // movie | tv | ""
	TmdbID         string
	MatchStatus    string
}

// FlixPatrolLinksOf returns the FlixPatrol titles linked to Netflix titles.
func (r *MappingRepository) FlixPatrolLinksOf(ctx context.Context, netflixIDs []int64) ([]LinkedFlixPatrol, error) {
	if len(netflixIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListTitleLinksForNetflixIDs(ctx, netflixIDs)
	if err != nil {
		return nil, fmt.Errorf("list title links for netflix titles: %w", err)
	}
	out := make([]LinkedFlixPatrol, 0, len(rows))
	for _, l := range rows {
		out = append(out, LinkedFlixPatrol{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID, Name: l.Name, Slug: l.Slug,
			Kind: l.Kind, TmdbID: l.TmdbID.String, MatchStatus: l.MatchStatus})
	}
	return out, nil
}

// NetflixLinksOf returns the Netflix titles linked to FlixPatrol titles.
func (r *MappingRepository) NetflixLinksOf(ctx context.Context, titleIDs []int64) ([]LinkedNetflix, error) {
	if len(titleIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListTitleLinksForFlixPatrolIDs(ctx, titleIDs)
	if err != nil {
		return nil, fmt.Errorf("list title links for flixpatrol titles: %w", err)
	}
	out := make([]LinkedNetflix, 0, len(rows))
	for _, l := range rows {
		out = append(out, LinkedNetflix{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID, ShowTitle: l.ShowTitle, Kind: l.Kind,
			TmdbKind: l.TmdbKind.String, TmdbID: l.TmdbID.String, MatchStatus: l.MatchStatus})
	}
	return out, nil
}

// Remap moves every FlixPatrol and Netflix title mapped to from onto to,
// marking them fixed by hand, in one transaction. Returns the IDs moved.
func (r *MappingRepository) Remap(ctx context.Context, from, to TmdbRef) (titleIDs, netflixIDs []int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	titleIDs, err = q.RemapFlixPatrolTitles(ctx, sqlc.RemapFlixPatrolTitlesParams{
		NewKind: titleKindOf(to.Kind), NewTmdbID: nullIfEmpty(to.ID), OldKind: titleKindOf(from.Kind), OldTmdbID: nullIfEmpty(from.ID),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("remap flixpatrol titles: %w", err)
	}
	netflixIDs, err = q.RemapNetflixTitles(ctx, sqlc.RemapNetflixTitlesParams{
		NewTmdbKind: nullIfEmpty(to.Kind), NewTmdbID: nullIfEmpty(to.ID), OldTmdbKind: nullIfEmpty(from.Kind), OldTmdbID: nullIfEmpty(from.ID),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("remap netflix titles: %w", err)
	}
	return titleIDs, netflixIDs, tx.Commit()
}

// titleKindOf maps a TMDB kind (movie | tv) to titles.kind (movie | tv_show).
func titleKindOf(tmdbKind string) string {
	if tmdbKind == "tv" {
		return "tv_show"
	}
	return "movie"
}

// ClearAdoptedFromFlixPatrol clears the linked Netflix titles that took their
// mapping from a FlixPatrol title, returning their IDs.
func (r *MappingRepository) ClearAdoptedFromFlixPatrol(ctx context.Context, titleID int64) ([]int64, error) {
	ids, err := r.q.ClearNetflixTitlesAdoptedFrom(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("clear netflix titles adopted from %d: %w", titleID, err)
	}
	return ids, nil
}

// ClearAdoptedFromNetflix clears the linked FlixPatrol titles that took their
// mapping from a Netflix title.
func (r *MappingRepository) ClearAdoptedFromNetflix(ctx context.Context, netflixTitleID int64) error {
	if err := r.q.ClearFlixPatrolTitlesAdoptedFrom(ctx, netflixTitleID); err != nil {
		return fmt.Errorf("clear flixpatrol titles adopted from %d: %w", netflixTitleID, err)
	}
	return nil
}
