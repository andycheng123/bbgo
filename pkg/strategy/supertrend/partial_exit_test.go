package supertrend

import (
	"context"
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestShouldTriggerPartialExit(t *testing.T) {
	entry := fixedpoint.NewFromFloat(100)
	atr := 2.0
	multiple := 1.5

	cases := []struct {
		name string
		base fixedpoint.Value
		high fixedpoint.Value
		low  fixedpoint.Value
		atr  float64
		want bool
	}{
		{"long trigger", fixedpoint.NewFromFloat(1), fixedpoint.NewFromFloat(103.1), fixedpoint.NewFromFloat(99), atr, true},
		{"long boundary", fixedpoint.NewFromFloat(1), fixedpoint.NewFromFloat(103), fixedpoint.NewFromFloat(99), atr, true},
		{"long no trigger", fixedpoint.NewFromFloat(1), fixedpoint.NewFromFloat(102.9), fixedpoint.NewFromFloat(99), atr, false},
		{"short trigger", fixedpoint.NewFromFloat(-1), fixedpoint.NewFromFloat(101), fixedpoint.NewFromFloat(96.9), atr, true},
		{"short boundary", fixedpoint.NewFromFloat(-1), fixedpoint.NewFromFloat(101), fixedpoint.NewFromFloat(97), atr, true},
		{"short no trigger", fixedpoint.NewFromFloat(-1), fixedpoint.NewFromFloat(101), fixedpoint.NewFromFloat(97.1), atr, false},
		{"zero base no trigger", fixedpoint.Zero, fixedpoint.NewFromFloat(103.1), fixedpoint.NewFromFloat(96.9), atr, false},
		{"zero atr no trigger", fixedpoint.NewFromFloat(1), fixedpoint.NewFromFloat(103.1), fixedpoint.NewFromFloat(99), 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var k types.KLine
			k.High = c.high
			k.Low = c.low
			got := shouldTriggerPartialExit(k, c.base, entry, c.atr, multiple)
			if got != c.want {
				t.Fatalf("shouldTriggerPartialExit()=%v want %v", got, c.want)
			}
		})
	}
}

func TestPartialExitValidateDefaults(t *testing.T) {
	pe := &PartialExit{}
	if err := pe.Validate(); err != nil {
		t.Fatalf("default PartialExit should validate, got %v", err)
	}
	if pe.AtrMultiple != 1.0 {
		t.Fatalf("expected default AtrMultiple 1.0, got %v", pe.AtrMultiple)
	}
	if pe.LockRatio.Compare(fixedpoint.NewFromFloat(0.5)) != 0 {
		t.Fatalf("expected default LockRatio 0.5, got %v", pe.LockRatio)
	}
	if pe.AtrWindow != 14 {
		t.Fatalf("expected default AtrWindow 14, got %d", pe.AtrWindow)
	}
}

func TestPartialExitValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		pe   *PartialExit
	}{
		{"negative atr multiple", &PartialExit{AtrMultiple: -1, LockRatio: fixedpoint.NewFromFloat(0.5), AtrWindow: 14}},
		{"negative lock ratio", &PartialExit{AtrMultiple: 1, LockRatio: fixedpoint.NewFromFloat(-0.1), AtrWindow: 14}},
		{"one lock ratio", &PartialExit{AtrMultiple: 1, LockRatio: fixedpoint.One, AtrWindow: 14}},
		{"negative atr window", &PartialExit{AtrMultiple: 1, LockRatio: fixedpoint.NewFromFloat(0.5), AtrWindow: -1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.pe.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPartialExitNilSafe(t *testing.T) {
	var pe *PartialExit
	if err := pe.Validate(); err != nil {
		t.Fatalf("nil PartialExit should validate, got %v", err)
	}
	pe.Reset()
	pe.Arm(types.KLine{}, fixedpoint.NewFromFloat(100))
	pe.pushK(types.KLine{})
	pe.setupIndicators(nil, nil)
	pe.CloseIfTriggered(context.Background(), nil, types.KLine{}, fixedpoint.NewFromFloat(100), false)
}
