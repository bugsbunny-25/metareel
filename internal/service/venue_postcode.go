package service

import (
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// Postcodes (ZIP, PIN, postcode) on cinemas: read from OSM addr:postcode,
// searchable, and used to fill a cinema's city and region (state) when OSM
// has neither on it.

// normPostcode tidies a postcode: single spaces, upper case ("sw1a 1aa" ->
// "SW1A 1AA"); nil stays nil and blank becomes nil.
func normPostcode(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.ToUpper(strings.Join(strings.Fields(*p), " "))
	if v == "" {
		return nil
	}
	return &v
}

// osmPostcode is an element's addr:postcode, tidied and capped.
func osmPostcode(t map[string]string) *string {
	v := t["addr:postcode"]
	p := normPostcode(&v)
	if p == nil || len([]rune(*p)) > 20 {
		return nil
	}
	return p
}

// postcodeIndex knows, for each postcode seen on OSM features of a country,
// the city and region they most often name.
type postcodeIndex struct {
	city, region map[string]map[string]int
}

func newPostcodeIndex(sets ...[]client.OSMPlace) *postcodeIndex {
	ix := &postcodeIndex{city: map[string]map[string]int{}, region: map[string]map[string]int{}}
	add := func(m map[string]map[string]int, pc string, v *string) {
		if v == nil {
			return
		}
		if m[pc] == nil {
			m[pc] = map[string]int{}
		}
		m[pc][*v]++
	}
	for _, places := range sets {
		for _, p := range places {
			pc := osmPostcode(p.Tags)
			if pc == nil {
				continue
			}
			add(ix.city, *pc, tagPtr(p.Tags, "addr:city"))
			region := tagPtr(p.Tags, "addr:state")
			if region == nil {
				region = tagPtr(p.Tags, "addr:province")
			}
			add(ix.region, *pc, region)
		}
	}
	return ix
}

// lookup is the city and region most named with a postcode (nil when none
// was seen, or a tie leaves it unclear).
func (ix *postcodeIndex) lookup(pc string) (city, region *string) {
	if ix == nil {
		return nil, nil
	}
	return mostNamed(ix.city[pc]), mostNamed(ix.region[pc])
}

func mostNamed(counts map[string]int) *string {
	best, n, tie := "", 0, false
	for v, c := range counts {
		switch {
		case c > n:
			best, n, tie = v, c, false
		case c == n:
			tie = true
		}
	}
	if n == 0 || tie {
		return nil
	}
	return &best
}

// regionForPostcode is the state a postcode alone tells, where that is fixed
// and unambiguous: US ZIP codes, by their first three digits (USPS
// sectional centres), as the two-letter code OSM uses in addr:state. Other
// countries return nil: India's PIN prefixes cross state lines (403 is Goa
// inside Maharashtra's 40-44), and Britain's postcodes have no states.
func regionForPostcode(country, postcode string) *string {
	if country != "US" || len(postcode) < 5 {
		return nil
	}
	n, err := strconv.Atoi(postcode[:3])
	if err != nil {
		return nil
	}
	for _, r := range usZIP3 {
		if n >= r.from && n <= r.to {
			s := r.state
			return &s
		}
	}
	return nil
}

// usZIP3 maps ZIP prefixes to states. Military (AA, AE, AP) and territory
// prefixes are left out.
var usZIP3 = []struct {
	from, to int
	state    string
}{
	{5, 5, "NY"}, {10, 27, "MA"}, {28, 29, "RI"}, {30, 38, "NH"}, {39, 49, "ME"},
	{50, 54, "VT"}, {55, 55, "MA"}, {56, 59, "VT"}, {60, 69, "CT"}, {70, 89, "NJ"},
	{100, 149, "NY"}, {150, 196, "PA"}, {197, 199, "DE"}, {200, 200, "DC"}, {201, 201, "VA"},
	{202, 205, "DC"}, {206, 219, "MD"}, {220, 246, "VA"}, {247, 268, "WV"}, {270, 289, "NC"},
	{290, 299, "SC"}, {300, 319, "GA"}, {320, 339, "FL"}, {341, 349, "FL"}, {350, 369, "AL"},
	{370, 385, "TN"}, {386, 397, "MS"}, {398, 399, "GA"}, {400, 427, "KY"}, {430, 459, "OH"},
	{460, 479, "IN"}, {480, 499, "MI"}, {500, 528, "IA"}, {530, 549, "WI"}, {550, 567, "MN"},
	{569, 569, "DC"}, {570, 577, "SD"}, {580, 588, "ND"}, {590, 599, "MT"}, {600, 629, "IL"},
	{630, 658, "MO"}, {660, 679, "KS"}, {680, 693, "NE"}, {700, 714, "LA"}, {716, 729, "AR"},
	{730, 732, "OK"}, {733, 733, "TX"}, {734, 749, "OK"}, {750, 799, "TX"}, {800, 816, "CO"},
	{820, 831, "WY"}, {832, 838, "ID"}, {840, 847, "UT"}, {850, 865, "AZ"}, {870, 884, "NM"},
	{885, 885, "TX"}, {889, 898, "NV"}, {900, 961, "CA"}, {967, 968, "HI"}, {970, 979, "OR"},
	{980, 994, "WA"}, {995, 999, "AK"},
}
