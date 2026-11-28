package service

import (
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
)

func TestPostcodes(t *testing.T) {
	if p := normPostcode(new("  sw1a   1aa ")); p == nil || *p != "SW1A 1AA" {
		t.Fatalf("norm = %v", p)
	}
	if normPostcode(new("  ")) != nil || normPostcode(nil) != nil {
		t.Fatal("blank postcode kept")
	}
	if osmPostcode(map[string]string{"addr:postcode": "123456789012345678901"}) != nil {
		t.Fatal("over-long postcode kept")
	}
	for _, tc := range []struct{ country, zip, want string }{
		{"US", "10036", "NY"}, {"US", "02139", "MA"}, {"US", "05501", "MA"}, {"US", "05401", "VT"},
		{"US", "20101", "VA"}, {"US", "20500", "DC"}, {"US", "73301", "TX"}, {"US", "73102", "OK"},
		{"US", "90028", "CA"}, {"US", "99501", "AK"}, {"US", "96813", "HI"}, {"US", "88510", "TX"},
		{"US", "09001", ""}, {"US", "123", ""}, {"US", "ABCDE", ""}, {"IN", "400001", ""}, {"GB", "SW1A 1AA", ""},
	} {
		got := regionForPostcode(tc.country, tc.zip)
		if (got == nil) != (tc.want == "") || (got != nil && *got != tc.want) {
			t.Errorf("%s %s = %v, want %q", tc.country, tc.zip, got, tc.want)
		}
	}

	pl := func(tags ...string) client.OSMPlace {
		m := map[string]string{}
		for i := 0; i+1 < len(tags); i += 2 {
			m[tags[i]] = tags[i+1]
		}
		return client.OSMPlace{Type: "node", Tags: m}
	}
	ix := newPostcodeIndex(
		[]client.OSMPlace{pl("addr:postcode", "400050", "addr:city", "Mumbai", "addr:state", "Maharashtra"), pl("addr:postcode", "400050", "addr:city", "Mumbai")},
		[]client.OSMPlace{pl("addr:postcode", "400050", "addr:city", "Bandra"), pl("addr:postcode", "560001", "addr:city", "Bengaluru"), pl("addr:postcode", "560001", "addr:city", "Bangalore")},
	)
	if c, r := ix.lookup("400050"); c == nil || *c != "Mumbai" || r == nil || *r != "Maharashtra" {
		t.Fatalf("400050 = %v %v", c, r)
	}
	if c, r := ix.lookup("560001"); c != nil || r != nil {
		t.Fatalf("a tie names nothing: %v %v", c, r)
	}
	var none *postcodeIndex
	if c, r := none.lookup("x"); c != nil || r != nil {
		t.Fatal("nil index")
	}
}
