package service

import (
	"context"
	"math"
	"sort"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Cinemas near a point, for "which cinema am I in?" in the mobile app's log
// sheet. The phone sends its position; it is used for this one query and
// never stored or logged (the request logger redacts lat/lon).

const (
	defaultNearbyRadiusKm = 10.0
	maxNearbyRadiusKm     = 100.0
	minNearbyRadiusKm     = 0.1
	defaultNearbyLimit    = 20
	maxNearbyLimit        = 100
	// nearbyCandidateCap bounds how many cinemas the box query returns before
	// exact distances are computed; a 100 km box around the densest city in
	// the venue list holds well under this.
	nearbyCandidateCap = 5000
	// yourVenueBoostM moves a cinema the user has watched at ahead of
	// others up to this much closer, so the one you go to wins over its
	// neighbour in the same mall when GPS is a little off.
	yourVenueBoostM = 250.0
	kmPerDegreeLat  = 111.32
)

// NearbyVenue is a cinema with its distance from the asked-for point and the
// user's watches there.
type NearbyVenue struct {
	repository.Venue
	DistanceM     int64   `json:"distance_m"`
	YourWatches   int64   `json:"your_watches"`
	YourLastWatch *string `json:"your_last_watch,omitempty"`
}

// NearbyVenues lists cinemas within radiusKm (default 10, at most 100) of a
// point, nearest first, cinemas the user has watched at slightly favoured.
// Closed cinemas are left out unless includeClosed.
func (s *TrackingService) NearbyVenues(ctx context.Context, userID int64, lat, lon, radiusKm float64, limit int64, includeClosed bool) (map[string]any, error) {
	if math.IsNaN(lat) || lat < -90 || lat > 90 {
		return nil, &ValidationError{Message: "lat must be between -90 and 90"}
	}
	if math.IsNaN(lon) || lon < -180 || lon > 180 {
		return nil, &ValidationError{Message: "lon must be between -180 and 180"}
	}
	if radiusKm == 0 {
		radiusKm = defaultNearbyRadiusKm
	}
	if math.IsNaN(radiusKm) || radiusKm < minNearbyRadiusKm || radiusKm > maxNearbyRadiusKm {
		return nil, &ValidationError{Message: "radius_km must be between 0.1 and 100"}
	}
	if limit <= 0 {
		limit = defaultNearbyLimit
	}
	if limit > maxNearbyLimit {
		limit = maxNearbyLimit
	}

	box := nearbyBox(lat, lon, radiusKm)
	box.UserID, box.IncludeClosed, box.Limit = userID, includeClosed, nearbyCandidateCap
	rows, err := s.read.ListVenuesInBox(ctx, box)
	if err != nil {
		return nil, err
	}

	radiusM := radiusKm * 1000
	type ranked struct {
		v    NearbyVenue
		d, k float64
	}
	found := make([]ranked, 0, len(rows))
	for _, r := range rows {
		if r.Latitude == nil || r.Longitude == nil {
			continue
		}
		d := haversineM(lat, lon, *r.Latitude, *r.Longitude)
		if d > radiusM {
			continue
		}
		key := d
		if r.YourWatches > 0 {
			key -= yourVenueBoostM
		}
		nv := NearbyVenue{Venue: r.Venue, DistanceM: int64(math.Round(d)), YourWatches: r.YourWatches}
		if r.YourLastWatch != "" {
			last := r.YourLastWatch
			nv.YourLastWatch = &last
		}
		found = append(found, ranked{v: nv, d: d, k: key})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].k != found[j].k {
			return found[i].k < found[j].k
		}
		if found[i].d != found[j].d {
			return found[i].d < found[j].d
		}
		return found[i].v.ID < found[j].v.ID
	})
	if int64(len(found)) > limit {
		found = found[:limit]
	}
	out := make([]NearbyVenue, len(found))
	for i, f := range found {
		out[i] = f.v
	}
	return map[string]any{"venues": out, "radius_km": radiusKm, "limit": limit}, nil
}

// nearbyBox is the latitude/longitude box around a point that holds every
// spot within radiusKm. Near the poles, or when the box would cross the
// antimeridian, it spans every longitude (the exact distance check then
// does the work; that latitude band holds few cinemas).
func nearbyBox(lat, lon, radiusKm float64) repository.VenueBox {
	dLat := radiusKm / kmPerDegreeLat
	b := repository.VenueBox{
		MinLat: math.Max(lat-dLat, -90), MaxLat: math.Min(lat+dLat, 90),
		MinLon: -180, MaxLon: 180,
	}
	cosLat := math.Cos(lat * math.Pi / 180)
	if b.MinLat > -89 && b.MaxLat < 89 && cosLat > 0.01 {
		dLon := radiusKm / (kmPerDegreeLat * cosLat)
		if lon-dLon >= -180 && lon+dLon <= 180 {
			b.MinLon, b.MaxLon = lon-dLon, lon+dLon
		}
	}
	return b
}
