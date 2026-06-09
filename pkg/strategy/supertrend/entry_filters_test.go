package supertrend

import (
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/indicator"
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

// --- ADX filter ---

func TestADXAllows(t *testing.T) {
	if !adxAllows(25, 20) {
		t.Fatal("adx 25 >= 20 should pass")
	}
	if adxAllows(15, 20) {
		t.Fatal("adx 15 < 20 should veto")
	}
	if !adxAllows(20, 20) {
		t.Fatal("adx 20 >= 20 should pass (boundary)")
	}
}

func TestDIAllows(t *testing.T) {
	if !diAllows(30, 20, types.SideTypeBuy) {
		t.Fatal("buy with +DI>=-DI should pass")
	}
	if diAllows(20, 30, types.SideTypeBuy) {
		t.Fatal("buy with +DI<-DI should veto")
	}
	if !diAllows(20, 30, types.SideTypeSell) {
		t.Fatal("sell with -DI>=+DI should pass")
	}
	if diAllows(30, 20, types.SideTypeSell) {
		t.Fatal("sell with -DI<+DI should veto")
	}
	if !diAllows(10, 10, types.SideType("")) {
		t.Fatal("non-entry side should pass")
	}
}

func TestADXFilterNotReadyAllows(t *testing.T) {
	f := &ADXFilter{} // no dmi -> not ready -> allows all
	if !f.allows(types.SideTypeBuy) || !f.allows(types.SideTypeSell) {
		t.Fatal("not-ready ADX filter should not veto")
	}
}

func TestADXFilterValidate(t *testing.T) {
	if err := (&EntryFilters{ADX: &ADXFilter{}}).Validate(); err == nil {
		t.Fatal("expected error for adx with missing interval")
	}
	if err := (&EntryFilters{ADX: &ADXFilter{Interval: types.Interval5m, MinADX: 20}}).Validate(); err != nil {
		t.Fatalf("adx with interval should validate, got %v", err)
	}
	if err := (&EntryFilters{ADX: &ADXFilter{Interval: types.Interval5m, MinADX: -1}}).Validate(); err == nil {
		t.Fatal("expected error for negative minADX")
	}
}

// feedTrendingADX returns an ADXFilter whose DMI is warmed with a strong steady uptrend, which
// yields a high ADX and +DI > -DI.
func feedTrendingADX(window, smoothing int) *ADXFilter {
	f := &ADXFilter{Interval: types.Interval5m, Window: window, ADXSmoothing: smoothing}
	f.dmi = &indicator.DMI{
		IntervalWindow: types.IntervalWindow{Interval: f.Interval, Window: window},
		ADXSmoothing:   smoothing,
	}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < window*4+20; i++ {
		p := 100.0 + float64(i)
		var k types.KLine
		k.High = fixedpoint.NewFromFloat(p + 0.5)
		k.Low = fixedpoint.NewFromFloat(p - 0.5)
		k.Close = fixedpoint.NewFromFloat(p)
		k.EndTime = types.Time(base.Add(time.Duration(i) * 5 * time.Minute))
		f.pushK(k)
	}
	return f
}

func TestADXFilterThreshold(t *testing.T) {
	f := feedTrendingADX(14, 14)
	if !f.ready() {
		t.Fatal("expected DMI to be ready after warmup")
	}
	f.MinADX = 0
	if !f.allows(types.SideTypeBuy) {
		t.Fatal("MinADX=0 should allow any entry")
	}
	f.MinADX = 1000 // ADX is bounded ~[0,100]; an impossible threshold must veto
	if f.allows(types.SideTypeBuy) {
		t.Fatal("MinADX=1000 should veto (ADX cannot reach it)")
	}
}

func TestADXFilterDIAlignment(t *testing.T) {
	f := feedTrendingADX(14, 14)
	f.MinADX = 0
	f.RequireDIAlignment = true
	if !f.allows(types.SideTypeBuy) {
		t.Fatal("strong uptrend should allow long with DI alignment")
	}
	if f.allows(types.SideTypeSell) {
		t.Fatal("strong uptrend should veto short with DI alignment")
	}
}

func TestADXFilterPushKDedup(t *testing.T) {
	f := feedTrendingADX(14, 14)
	n := f.dmi.Length()
	// re-pushing an older kline must be ignored by the end-time guard
	var k types.KLine
	k.High = fixedpoint.NewFromFloat(50)
	k.Low = fixedpoint.NewFromFloat(49)
	k.Close = fixedpoint.NewFromFloat(49.5)
	k.EndTime = types.Time(time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC))
	f.pushK(k)
	if f.dmi.Length() != n {
		t.Fatalf("stale kline should be ignored; length changed %d -> %d", n, f.dmi.Length())
	}
}
