package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// DiscsRepository stores physical media: shared releases (with their box-set
// parts, titles and images), users' copies and the barcode lookup cache.
type DiscsRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewDiscsRepository(db *sql.DB) *DiscsRepository {
	return &DiscsRepository{db: db, q: sqlc.New(db)}
}

// DiscRelease is an edition as published. Discs and SpecialFeatures are JSON
// documents the service layer decodes.
type DiscRelease struct {
	ID              int64
	ParentID        *int64
	Position        int64
	Barcode         *string
	Title           string
	Edition         string
	MediaFormat     string
	Packaging       string
	HasSlipcover    bool
	Region          string
	Country         *string
	Studio          string
	Label           *string
	ReleaseDate     *string
	Discs           string
	SpecialFeatures string
	Notes           *string
	Source          string
	CreatedAt       string
	UpdatedAt       string
}

func discReleaseFrom(r sqlc.DiscRelease) DiscRelease {
	return DiscRelease{
		ID: r.ID, ParentID: nullInt64Ptr(r.ParentID), Position: r.Position, Barcode: nullStringPtr(r.Barcode),
		Title: r.Title, Edition: r.Edition, MediaFormat: r.MediaFormat, Packaging: r.Packaging,
		HasSlipcover: r.HasSlipcover != 0, Region: r.Region, Country: nullStringPtr(r.Country),
		Studio: r.Studio, Label: nullStringPtr(r.Label), ReleaseDate: nullStringPtr(r.ReleaseDate),
		Discs: r.Discs, SpecialFeatures: r.SpecialFeatures, Notes: nullStringPtr(r.Notes),
		Source: r.Source, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// DiscReleaseTitle is one title on a release, with its display snapshot.
type DiscReleaseTitle struct {
	ReleaseID    int64
	Position     int64
	Kind         string
	ID           string
	SeasonNumber *int64
	Title        string
	ReleaseDate  string
	PosterPath   string
}

// DiscReleaseImage is one picture of a release.
type DiscReleaseImage struct {
	ID          int64
	ReleaseID   int64
	Kind        string
	Position    int64
	SHA256      string
	ContentType string
	Width       int64
	Height      int64
	Bytes       int64
	Source      string
	SourceURL   *string
	CreatedAt   string
}

func discImageFrom(r sqlc.DiscReleaseImage) DiscReleaseImage {
	return DiscReleaseImage{
		ID: r.ID, ReleaseID: r.ReleaseID, Kind: r.Kind, Position: r.Position, SHA256: r.Sha256, ContentType: r.ContentType,
		Width: r.Width, Height: r.Height, Bytes: r.Bytes, Source: r.Source,
		SourceURL: nullStringPtr(r.SourceUrl), CreatedAt: r.CreatedAt,
	}
}

// DiscCopy is a user's copy of a release.
type DiscCopy struct {
	ID          int64
	UserID      int64
	ReleaseID   int64
	PurchasedOn *string
	PriceMinor  *int64
	Currency    *string
	Retailer    string
	Condition   string
	Notes       *string
	CreatedAt   string
	UpdatedAt   string
}

func discCopyFrom(r sqlc.DiscCopy) DiscCopy {
	return DiscCopy{
		ID: r.ID, UserID: r.UserID, ReleaseID: r.ReleaseID, PurchasedOn: nullStringPtr(r.PurchasedOn),
		PriceMinor: nullInt64Ptr(r.PriceMinor), Currency: nullStringPtr(r.Currency), Retailer: r.Retailer,
		Condition: r.Condition, Notes: nullStringPtr(r.Notes), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// DiscCopyRow is a copy on a shelf listing with its release's headline fields.
type DiscCopyRow struct {
	DiscCopy
	Title        string
	Edition      string
	MediaFormat  string
	Packaging    string
	Barcode      *string
	Label        *string
	Studio       string
	HasSlipcover bool
	Discs        string // JSON
}

// ReleaseTitleRef is a title to put on a release.
type ReleaseTitleRef struct {
	Kind         string
	ID           string
	SeasonNumber *int64
}

// ReleaseWrite is a release to create (ID 0) or update, with its titles and,
// for a box set, its parts. Parts are matched to stored parts by ID; a stored
// part missing from Parts is deleted.
type ReleaseWrite struct {
	ID              int64
	Position        int64
	Barcode         *string
	Title           string
	Edition         string
	MediaFormat     string
	Packaging       string
	HasSlipcover    bool
	Region          string
	Country         *string
	Studio          string
	Label           *string
	ReleaseDate     *string
	Discs           string
	SpecialFeatures string
	Notes           *string
	Source          string
	Titles          []ReleaseTitleRef
	Parts           []ReleaseWrite
}

// ErrDiscBarcodeTaken is returned when another release already has the barcode.
var ErrDiscBarcodeTaken = errors.New("another release has this barcode")

// SaveReleaseTree writes a release with its titles and parts, and with
// addCopy also the user's copy of it, in one transaction. It returns the
// release's id, the copy's id (0 without addCopy) and the image files of
// deleted parts, which the caller removes once nothing else uses them.
func (r *DiscsRepository) SaveReleaseTree(ctx context.Context, userID int64, w ReleaseWrite, addCopy *CopyWrite) (int64, int64, []string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	id, err := saveRelease(ctx, q, userID, nil, w)
	if err != nil {
		return 0, 0, nil, err
	}
	stored, err := q.ListDiscReleaseParts(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return 0, 0, nil, fmt.Errorf("list parts: %w", err)
	}
	keep := map[int64]bool{}
	for i, p := range w.Parts {
		if p.ID != 0 && !partOf(stored, p.ID) {
			return 0, 0, nil, fmt.Errorf("part %d: %w", p.ID, sql.ErrNoRows)
		}
		p.Position = int64(i)
		pid, err := saveRelease(ctx, q, userID, &id, p)
		if err != nil {
			return 0, 0, nil, err
		}
		keep[pid] = true
	}
	var removed []string
	for _, s := range stored {
		if keep[s.ID] {
			continue
		}
		shas, err := q.ListDiscImageShasForTree(ctx, s.ID)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("part images: %w", err)
		}
		removed = append(removed, shas...)
		if err := q.DeleteDiscRelease(ctx, s.ID); err != nil {
			return 0, 0, nil, fmt.Errorf("delete part %d: %w", s.ID, err)
		}
	}
	var copyID int64
	if addCopy != nil {
		copyID, err = q.InsertDiscCopy(ctx, sqlc.InsertDiscCopyParams{
			UserID: userID, ReleaseID: id, PurchasedOn: toNullString(addCopy.PurchasedOn), PriceMinor: toNullInt64(addCopy.PriceMinor),
			Currency: toNullString(addCopy.Currency), Retailer: addCopy.Retailer, Condition: addCopy.Condition, Notes: toNullString(addCopy.Notes),
		})
		if err != nil {
			return 0, 0, nil, fmt.Errorf("insert copy: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, nil, fmt.Errorf("commit: %w", err)
	}
	return id, copyID, removed, nil
}

func partOf(parts []sqlc.DiscRelease, id int64) bool {
	for _, p := range parts {
		if p.ID == id {
			return true
		}
	}
	return false
}

func saveRelease(ctx context.Context, q *sqlc.Queries, userID int64, parent *int64, w ReleaseWrite) (int64, error) {
	if w.Barcode != nil {
		other, err := q.GetDiscReleaseByBarcode(ctx, sql.NullString{String: *w.Barcode, Valid: true})
		switch {
		case err == nil && other.ID != w.ID:
			return 0, ErrDiscBarcodeTaken
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return 0, fmt.Errorf("barcode check: %w", err)
		}
	}
	slip := int64(0)
	if w.HasSlipcover {
		slip = 1
	}
	id := w.ID
	if id == 0 {
		var err error
		id, err = q.InsertDiscRelease(ctx, sqlc.InsertDiscReleaseParams{
			ParentID: toNullInt64(parent), Position: w.Position, Barcode: toNullString(w.Barcode), Title: w.Title,
			Edition: w.Edition, MediaFormat: w.MediaFormat, Packaging: w.Packaging, HasSlipcover: slip,
			Region: w.Region, Country: toNullString(w.Country), Studio: w.Studio, Label: toNullString(w.Label),
			ReleaseDate: toNullString(w.ReleaseDate), Discs: w.Discs, SpecialFeatures: w.SpecialFeatures,
			Notes: toNullString(w.Notes), Source: w.Source, CreatedBy: sql.NullInt64{Int64: userID, Valid: userID != 0},
		})
		if err != nil {
			return 0, fmt.Errorf("insert release: %w", err)
		}
	} else {
		n, err := q.UpdateDiscRelease(ctx, sqlc.UpdateDiscReleaseParams{
			ID: id, Position: w.Position, Barcode: toNullString(w.Barcode), Title: w.Title, Edition: w.Edition,
			MediaFormat: w.MediaFormat, Packaging: w.Packaging, HasSlipcover: slip, Region: w.Region,
			Country: toNullString(w.Country), Studio: w.Studio, Label: toNullString(w.Label),
			ReleaseDate: toNullString(w.ReleaseDate), Discs: w.Discs, SpecialFeatures: w.SpecialFeatures,
			Notes: toNullString(w.Notes),
		})
		if err != nil {
			return 0, fmt.Errorf("update release %d: %w", id, err)
		}
		if n == 0 {
			return 0, sql.ErrNoRows
		}
	}
	if err := q.DeleteDiscReleaseTitles(ctx, id); err != nil {
		return 0, fmt.Errorf("clear titles: %w", err)
	}
	for i, t := range w.Titles {
		if err := q.InsertDiscReleaseTitle(ctx, sqlc.InsertDiscReleaseTitleParams{
			ReleaseID: id, Position: int64(i), TmdbKind: t.Kind, TmdbID: t.ID, SeasonNumber: toNullInt64(t.SeasonNumber),
		}); err != nil {
			return 0, fmt.Errorf("insert title: %w", err)
		}
	}
	return id, nil
}

// Release returns one release (top-level or a part).
func (r *DiscsRepository) Release(ctx context.Context, id int64) (DiscRelease, error) {
	row, err := r.q.GetDiscRelease(ctx, id)
	if err != nil {
		return DiscRelease{}, err
	}
	return discReleaseFrom(row), nil
}

// ReleaseByBarcode returns the release with a barcode, or sql.ErrNoRows.
func (r *DiscsRepository) ReleaseByBarcode(ctx context.Context, barcode string) (DiscRelease, error) {
	row, err := r.q.GetDiscReleaseByBarcode(ctx, sql.NullString{String: barcode, Valid: true})
	if err != nil {
		return DiscRelease{}, err
	}
	return discReleaseFrom(row), nil
}

// Parts returns the parts of several box sets, keyed by parent id.
func (r *DiscsRepository) Parts(ctx context.Context, parentIDs []int64) (map[int64][]DiscRelease, error) {
	out := map[int64][]DiscRelease{}
	if len(parentIDs) == 0 {
		return out, nil
	}
	ids := make([]sql.NullInt64, len(parentIDs))
	for i, id := range parentIDs {
		ids[i] = sql.NullInt64{Int64: id, Valid: true}
	}
	rows, err := r.q.ListDiscReleasesByParents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	for _, row := range rows {
		out[row.ParentID.Int64] = append(out[row.ParentID.Int64], discReleaseFrom(row))
	}
	return out, nil
}

// Titles returns what is on several releases, keyed by release id.
func (r *DiscsRepository) Titles(ctx context.Context, releaseIDs []int64) (map[int64][]DiscReleaseTitle, error) {
	out := map[int64][]DiscReleaseTitle{}
	if len(releaseIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListDiscReleaseTitles(ctx, releaseIDs)
	if err != nil {
		return nil, fmt.Errorf("list release titles: %w", err)
	}
	for _, row := range rows {
		out[row.ReleaseID] = append(out[row.ReleaseID], DiscReleaseTitle{
			ReleaseID: row.ReleaseID, Position: row.Position, Kind: row.TmdbKind, ID: row.TmdbID,
			SeasonNumber: nullInt64Ptr(row.SeasonNumber), Title: row.Title, ReleaseDate: row.ReleaseDate,
			PosterPath: row.PosterPath,
		})
	}
	return out, nil
}

// Images returns the pictures of several releases, keyed by release id.
func (r *DiscsRepository) Images(ctx context.Context, releaseIDs []int64) (map[int64][]DiscReleaseImage, error) {
	out := map[int64][]DiscReleaseImage{}
	if len(releaseIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListDiscReleaseImages(ctx, releaseIDs)
	if err != nil {
		return nil, fmt.Errorf("list release images: %w", err)
	}
	for _, row := range rows {
		out[row.ReleaseID] = append(out[row.ReleaseID], discImageFrom(row))
	}
	return out, nil
}

// NewDiscImage is a stored picture to record.
type NewDiscImage struct {
	ReleaseID   int64
	Kind        string
	SHA256      string
	ContentType string
	Width       int64
	Height      int64
	Bytes       int64
	Source      string
	SourceURL   *string
	UploadedBy  int64
}

// AddImage records a picture after the kind's existing ones.
func (r *DiscsRepository) AddImage(ctx context.Context, in NewDiscImage) (DiscReleaseImage, error) {
	pos, err := r.q.MaxDiscImagePosition(ctx, sqlc.MaxDiscImagePositionParams{ReleaseID: in.ReleaseID, Kind: in.Kind})
	if err != nil {
		return DiscReleaseImage{}, fmt.Errorf("image position: %w", err)
	}
	row, err := r.q.InsertDiscReleaseImage(ctx, sqlc.InsertDiscReleaseImageParams{
		ReleaseID: in.ReleaseID, Kind: in.Kind, Position: pos + 1, Sha256: in.SHA256, ContentType: in.ContentType, Width: in.Width,
		Height: in.Height, Bytes: in.Bytes, Source: in.Source, SourceUrl: toNullString(in.SourceURL),
		UploadedBy: sql.NullInt64{Int64: in.UploadedBy, Valid: in.UploadedBy != 0},
	})
	if err != nil {
		return DiscReleaseImage{}, fmt.Errorf("insert image: %w", err)
	}
	return discImageFrom(row), nil
}

// Image returns one picture of a release, or sql.ErrNoRows.
func (r *DiscsRepository) Image(ctx context.Context, releaseID, id int64) (DiscReleaseImage, error) {
	row, err := r.q.GetDiscReleaseImage(ctx, sqlc.GetDiscReleaseImageParams{ID: id, ReleaseID: releaseID})
	if err != nil {
		return DiscReleaseImage{}, err
	}
	return discImageFrom(row), nil
}

// UpdateImage changes a picture's kind and position.
func (r *DiscsRepository) UpdateImage(ctx context.Context, releaseID, id int64, kind string, position int64) error {
	n, err := r.q.UpdateDiscReleaseImage(ctx, sqlc.UpdateDiscReleaseImageParams{Kind: kind, Position: position, ID: id, ReleaseID: releaseID})
	if err != nil {
		return fmt.Errorf("update image: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteImage forgets a picture.
func (r *DiscsRepository) DeleteImage(ctx context.Context, releaseID, id int64) error {
	n, err := r.q.DeleteDiscReleaseImage(ctx, sqlc.DeleteDiscReleaseImageParams{ID: id, ReleaseID: releaseID})
	if err != nil {
		return fmt.Errorf("delete image: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ImageFileInUse reports whether any picture still uses a stored file.
func (r *DiscsRepository) ImageFileInUse(ctx context.Context, sha string) (bool, error) {
	n, err := r.q.CountDiscImagesBySha(ctx, sha)
	return n > 0, err
}

// CopyWrite is the user-owned part of a copy.
type CopyWrite struct {
	PurchasedOn *string
	PriceMinor  *int64
	Currency    *string
	Retailer    string
	Condition   string
	Notes       *string
}

// AddCopy records a user's copy of a release.
func (r *DiscsRepository) AddCopy(ctx context.Context, userID, releaseID int64, w CopyWrite) (int64, error) {
	id, err := r.q.InsertDiscCopy(ctx, sqlc.InsertDiscCopyParams{
		UserID: userID, ReleaseID: releaseID, PurchasedOn: toNullString(w.PurchasedOn), PriceMinor: toNullInt64(w.PriceMinor),
		Currency: toNullString(w.Currency), Retailer: w.Retailer, Condition: w.Condition, Notes: toNullString(w.Notes),
	})
	if err != nil {
		return 0, fmt.Errorf("insert copy: %w", err)
	}
	return id, nil
}

// UpdateCopy changes a user's copy; sql.ErrNoRows when it isn't theirs.
func (r *DiscsRepository) UpdateCopy(ctx context.Context, userID, id int64, w CopyWrite) error {
	n, err := r.q.UpdateDiscCopy(ctx, sqlc.UpdateDiscCopyParams{
		ID: id, UserID: userID, PurchasedOn: toNullString(w.PurchasedOn), PriceMinor: toNullInt64(w.PriceMinor),
		Currency: toNullString(w.Currency), Retailer: w.Retailer, Condition: w.Condition, Notes: toNullString(w.Notes),
	})
	if err != nil {
		return fmt.Errorf("update copy: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteCopy removes a user's copy; the release stays as reference data.
func (r *DiscsRepository) DeleteCopy(ctx context.Context, userID, id int64) error {
	n, err := r.q.DeleteDiscCopy(ctx, sqlc.DeleteDiscCopyParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("delete copy: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Copy returns a user's copy, or sql.ErrNoRows.
func (r *DiscsRepository) Copy(ctx context.Context, userID, id int64) (DiscCopy, error) {
	row, err := r.q.GetDiscCopy(ctx, sqlc.GetDiscCopyParams{ID: id, UserID: userID})
	if err != nil {
		return DiscCopy{}, err
	}
	return discCopyFrom(row), nil
}

// CopiesOfRelease returns a user's copies of one release.
func (r *DiscsRepository) CopiesOfRelease(ctx context.Context, userID, releaseID int64) ([]DiscCopy, error) {
	rows, err := r.q.ListDiscCopiesOfRelease(ctx, sqlc.ListDiscCopiesOfReleaseParams{UserID: userID, ReleaseID: releaseID})
	if err != nil {
		return nil, fmt.Errorf("list copies of release: %w", err)
	}
	out := make([]DiscCopy, len(rows))
	for i, row := range rows {
		out[i] = discCopyFrom(row)
	}
	return out, nil
}

// ReleaseCopyCount is how many copies (any user's) point at a release.
func (r *DiscsRepository) ReleaseCopyCount(ctx context.Context, releaseID int64) (int64, error) {
	return r.q.CountDiscReleaseCopies(ctx, releaseID)
}

// CopyFilter narrows a shelf listing.
type CopyFilter struct {
	MediaFormat string
	Query       string
	TmdbKind    string
	TmdbID      string
	Sort        string // added | title | purchased | price | format
}

// Copies lists a user's shelf.
func (r *DiscsRepository) Copies(ctx context.Context, userID int64, f CopyFilter) ([]DiscCopyRow, error) {
	rows, err := r.q.ListDiscCopies(ctx, sqlc.ListDiscCopiesParams{
		Sort: f.Sort, UserID: userID, MediaFormat: f.MediaFormat, Q: f.Query, TmdbKind: f.TmdbKind, TmdbID: f.TmdbID,
	})
	if err != nil {
		return nil, fmt.Errorf("list copies: %w", err)
	}
	out := make([]DiscCopyRow, len(rows))
	for i, row := range rows {
		out[i] = DiscCopyRow{
			ID: row.ID, UserID: row.UserID, ReleaseID: row.ReleaseID, PurchasedOn: nullStringPtr(row.PurchasedOn),
			PriceMinor: nullInt64Ptr(row.PriceMinor), Currency: nullStringPtr(row.Currency), Retailer: row.Retailer,
			Condition: row.Condition, Notes: nullStringPtr(row.Notes), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Title: row.Title, Edition: row.Edition, MediaFormat: row.MediaFormat, Packaging: row.Packaging,
			Barcode: nullStringPtr(row.Barcode), Label: nullStringPtr(row.Label), Studio: row.Studio,
			HasSlipcover: row.HasSlipcover != 0, Discs: row.Discs,
		}
	}
	return out, nil
}

// BarcodeLookup is a cached answer from a barcode service.
type BarcodeLookup struct {
	Found     bool
	Payload   string
	FetchedAt string
}

// CachedLookup returns a cached lookup, or sql.ErrNoRows.
func (r *DiscsRepository) CachedLookup(ctx context.Context, barcode, provider string) (BarcodeLookup, error) {
	row, err := r.q.GetBarcodeLookup(ctx, sqlc.GetBarcodeLookupParams{Barcode: barcode, Provider: provider})
	if err != nil {
		return BarcodeLookup{}, err
	}
	return BarcodeLookup{Found: row.Found != 0, Payload: row.Payload, FetchedAt: row.FetchedAt}, nil
}

// SaveLookup caches a barcode service's answer.
func (r *DiscsRepository) SaveLookup(ctx context.Context, barcode, provider string, found bool, payload string) error {
	f := int64(0)
	if found {
		f = 1
	}
	return r.q.SaveBarcodeLookup(ctx, sqlc.SaveBarcodeLookupParams{Barcode: barcode, Provider: provider, Found: f, Payload: payload})
}
