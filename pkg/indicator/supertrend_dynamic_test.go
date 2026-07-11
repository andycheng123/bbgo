package indicator

import (
	"encoding/json"
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func buildSupertrendDynamicTestKLines(t *testing.T, bytes []byte) []types.KLine {
	t.Helper()

	var prices map[string][]fixedpoint.Value
	if err := json.Unmarshal(bytes, &prices); err != nil {
		t.Fatal(err)
	}

	kLines := make([]types.KLine, 0, len(prices["high"]))
	for i, h := range prices["high"] {
		kLines = append(kLines, types.KLine{High: h, Low: prices["low"][i], Close: prices["close"][i]})
	}
	return kLines
}

func newTestSupertrend(window int, multiplier float64) *Supertrend {
	st := &Supertrend{
		IntervalWindow: types.IntervalWindow{Window: window},
		ATRMultiplier:  multiplier,
	}
	st.AverageTrueRange = &ATR{IntervalWindow: types.IntervalWindow{Window: window}}
	return st
}

func newTestSupertrendDynamic(window int, multiplier float64) *SupertrendDynamic {
	st := &SupertrendDynamic{
		IntervalWindow: types.IntervalWindow{Window: window},
		ATRMultiplier:  multiplier,
	}
	st.AverageTrueRange = &ATR{IntervalWindow: types.IntervalWindow{Window: window}}
	return st
}

func TestSupertrendDynamic_ConstantMultiplierMatchesSupertrend(t *testing.T) {
	kLines := buildSupertrendDynamicTestKLines(t, []byte(`{
		"high": [101, 102, 103, 104, 105, 104, 103, 102, 101, 100, 99, 98, 99, 100, 101, 102, 103, 104, 103, 102, 101, 100],
		"low": [99, 100, 101, 102, 103, 101, 100, 99, 98, 97, 96, 95, 96, 97, 98, 99, 100, 101, 100, 99, 98, 97],
		"close": [100, 101, 102, 103, 104, 102, 101, 100, 99, 98, 97, 96, 98, 99, 100, 101, 102, 103, 101, 100, 99, 98]
	}`))

	const (
		window     = 5
		multiplier = 2.5
	)
	original := newTestSupertrend(window, multiplier)
	dynamic := newTestSupertrendDynamic(window, multiplier)

	for i, k := range kLines {
		original.PushK(k)
		dynamic.SetMultiplier(multiplier)
		dynamic.PushK(k)

		if got, want := dynamic.Last(0), original.Last(0); got != want {
			t.Fatalf("bar %d trend price = %v, want %v", i, got, want)
		}
		if got, want := dynamic.LastSupertrendSupport(), original.LastSupertrendSupport(); got != want {
			t.Fatalf("bar %d support = %v, want %v", i, got, want)
		}
		if got, want := dynamic.LastSupertrendResistance(), original.LastSupertrendResistance(); got != want {
			t.Fatalf("bar %d resistance = %v, want %v", i, got, want)
		}
		if got, want := dynamic.Direction(), original.Direction(); got != want {
			t.Fatalf("bar %d direction = %v, want %v", i, got, want)
		}
		if got, want := dynamic.GetSignal(), original.GetSignal(); got != want {
			t.Fatalf("bar %d signal = %v, want %v", i, got, want)
		}
	}
}

func TestSupertrendDynamic_ChangingMultiplierUpdatesBandsAndPreservesRatchet(t *testing.T) {
	kLines := buildSupertrendDynamicTestKLines(t, []byte(`{
		"high": [101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112],
		"low": [99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110],
		"close": [100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111]
	}`))

	const window = 3
	dynamic := newTestSupertrendDynamic(window, 3)
	constantHigh := newTestSupertrend(window, 3)

	previousSupport := 0.0
	responded := false
	for i, k := range kLines {
		multiplier := 3.0
		if i >= 7 {
			multiplier = 0.75
		}

		dynamic.SetMultiplier(multiplier)
		dynamic.PushK(k)
		constantHigh.PushK(k)

		if dynamic.Direction() != types.DirectionUp {
			t.Fatalf("bar %d direction = %v, want uptrend", i, dynamic.Direction())
		}

		support := dynamic.LastSupertrendSupport()
		if i > 2 && support < previousSupport {
			t.Fatalf("bar %d support ratchet moved down: %v < %v", i, support, previousSupport)
		}
		if i >= 7 && support > constantHigh.LastSupertrendSupport() {
			responded = true
		}
		previousSupport = support
	}

	if !responded {
		t.Fatal("dynamic support did not respond to the lower multiplier")
	}
}
