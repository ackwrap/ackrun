package model

import "testing"

func TestNormalizeGeoSource(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"", GeoSourceSagerNet}, {" SagerNet ", GeoSourceSagerNet}, {"Loyalsoldier", GeoSourceLoyalsoldier},
	} {
		got, err := NormalizeGeoSource(test.input)
		if err != nil || got != test.want {
			t.Fatalf("NormalizeGeoSource(%q)=(%q,%v), want %q", test.input, got, err, test.want)
		}
	}
	if _, err := NormalizeGeoSource("unknown"); err == nil {
		t.Fatal("unknown source accepted")
	}
}
