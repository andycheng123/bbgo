package supertrend

import (
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestHTFTrendAllows(t *testing.T) {
	cases := []struct {
		name  string
		trend types.Direction
		side  types.SideType
		want  bool
	}{
		{"uptrend allows long", types.DirectionUp, types.SideTypeBuy, true},
		{"uptrend vetoes short", types.DirectionUp, types.SideTypeSell, false},
		{"downtrend allows short", types.DirectionDown, types.SideTypeSell, true},
		{"downtrend vetoes long", types.DirectionDown, types.SideTypeBuy, false},
		{"no trend allows long", types.DirectionNone, types.SideTypeBuy, true},
		{"no trend allows short", types.DirectionNone, types.SideTypeSell, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := htfTrendAllows(c.trend, c.side); got != c.want {
				t.Fatalf("htfTrendAllows(%v,%v)=%v want %v", c.trend, c.side, got, c.want)
			}
		})
	}
}

func TestEntryFiltersFilterOpenSideNilSafe(t *testing.T) {
	var ef *EntryFilters
	if got := ef.FilterOpenSide(types.SideTypeBuy, types.KLine{}); got != types.SideTypeBuy {
		t.Fatalf("nil EntryFilters should pass side through, got %v", got)
	}
}

func TestEntryFiltersDisabledPassThrough(t *testing.T) {
	ef := &EntryFilters{} // no sub-filters
	for _, side := range []types.SideType{types.SideTypeBuy, types.SideTypeSell, types.SideType("")} {
		if got := ef.FilterOpenSide(side, types.KLine{}); got != side {
			t.Fatalf("empty EntryFilters should pass %v through, got %v", side, got)
		}
	}
}

func TestEntryFiltersNonEntrySidePassThrough(t *testing.T) {
	ef := &EntryFilters{HTFTrend: &HTFTrendFilter{}}
	// An empty side is not an entry; it must pass through untouched even with a filter present.
	if got := ef.FilterOpenSide(types.SideType(""), types.KLine{}); got != types.SideType("") {
		t.Fatalf("non-entry side should pass through, got %v", got)
	}
}

func TestHTFTrendFilterAllowsWithoutIndicator(t *testing.T) {
	f := &HTFTrendFilter{} // linReg nil -> signal None -> allows all
	if !f.allows(types.SideTypeBuy) || !f.allows(types.SideTypeSell) {
		t.Fatal("filter without a warmed indicator should not veto")
	}
}

// feedRisingLinReg drives a LinReg with strictly rising closes so its slope (and GetSignal) is Up.
func feedRisingLinReg(window int) *LinReg {
	lr := &LinReg{IntervalWindow: types.IntervalWindow{Interval: types.Interval5m, Window: window}}
	for i := 0; i < window*2; i++ {
		var k types.KLine
		k.Close = fixedpoint.NewFromFloat(100.0 + float64(i))
		lr.Update(k)
	}
	return lr
}

func TestHTFTrendFilterVetoesAgainstTrend(t *testing.T) {
	f := &HTFTrendFilter{Interval: types.Interval5m, Window: 5}
	f.linReg = feedRisingLinReg(f.Window)

	if f.signal() != types.DirectionUp {
		t.Fatalf("expected HTF uptrend, got %v", f.signal())
	}
	if f.allows(types.SideTypeSell) {
		t.Fatal("short entry should be vetoed in an HTF uptrend")
	}
	if !f.allows(types.SideTypeBuy) {
		t.Fatal("long entry should be allowed in an HTF uptrend")
	}
}

func TestEntryFiltersVetoReturnsEmptyAndCounts(t *testing.T) {
	f := &HTFTrendFilter{Interval: types.Interval5m, Window: 5}
	f.linReg = feedRisingLinReg(f.Window)
	ef := &EntryFilters{HTFTrend: f}

	if got := ef.FilterOpenSide(types.SideTypeSell, types.KLine{}); got != types.SideType("") {
		t.Fatalf("expected veto (empty side) for short against uptrend, got %v", got)
	}
	if got := ef.FilterOpenSide(types.SideTypeBuy, types.KLine{}); got != types.SideTypeBuy {
		t.Fatalf("expected long allowed in uptrend, got %v", got)
	}
	if f.vetoCount != 1 {
		t.Fatalf("expected vetoCount 1, got %d", f.vetoCount)
	}
}

func TestEntryFiltersValidate(t *testing.T) {
	var ef *EntryFilters
	if err := ef.Validate(); err != nil {
		t.Fatalf("nil EntryFilters should validate, got %v", err)
	}
	if err := (&EntryFilters{}).Validate(); err != nil {
		t.Fatalf("empty EntryFilters should validate, got %v", err)
	}
	if err := (&EntryFilters{HTFTrend: &HTFTrendFilter{}}).Validate(); err == nil {
		t.Fatal("expected error for htfTrend with missing interval")
	}
	if err := (&EntryFilters{HTFTrend: &HTFTrendFilter{Interval: types.Interval1h}}).Validate(); err != nil {
		t.Fatalf("htfTrend with interval should validate, got %v", err)
	}
}
