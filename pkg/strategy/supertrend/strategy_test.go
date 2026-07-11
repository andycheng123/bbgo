package supertrend

import (
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/datatype/floats"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/types/mocks"
)

func newTestSession(t *testing.T) *bbgo.ExchangeSession {
	t.Helper()

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)

	mockEx := mocks.NewMockExchange(mockCtrl)
	mockEx.EXPECT().NewStream().Return(&types.StandardStream{}).Times(2)
	mockEx.EXPECT().Name().Return(types.ExchangeName("backtest")).AnyTimes()

	return bbgo.NewExchangeSession("test", mockEx)
}

func TestStrategySubscribe_LinearRegressionNilSafe(t *testing.T) {
	session := newTestSession(t)
	s := &Strategy{
		Symbol:         "BTCUSDT",
		IntervalWindow: types.IntervalWindow{Interval: types.Interval1m, Window: 5},
		LinearRegression: &LinReg{
			IntervalWindow: types.IntervalWindow{Window: 0},
		},
	}

	s.Subscribe(session)

	if got, want := len(session.Subscriptions), 1; got != want {
		t.Fatalf("subscriptions = %d, want %d", got, want)
	}
}

func TestStrategySetupIndicators_SelectsStaticOrDynamicSupertrend(t *testing.T) {
	static := &Strategy{
		Symbol:               "BTCUSDT",
		IntervalWindow:       types.IntervalWindow{Interval: types.Interval1m, Window: 5},
		SupertrendMultiplier: 3,
		session:              newTestSession(t),
	}
	static.setupIndicators()

	if static.Supertrend == nil {
		t.Fatal("expected static supertrend")
	}
	if static.DynamicSupertrend != nil {
		t.Fatal("did not expect dynamic supertrend when adaptiveMultiplier is nil")
	}

	dynamic := &Strategy{
		Symbol:               "BTCUSDT",
		IntervalWindow:       types.IntervalWindow{Interval: types.Interval1m, Window: 5},
		SupertrendMultiplier: 3,
		AdaptiveMultiplier: &AdaptiveMultiplierConfig{
			PercentileWindow: 20,
			MLow:             2,
			MHigh:            4,
			Polarity:         adaptiveMultiplierPolarityHighVolHigh,
		},
		session: newTestSession(t),
	}
	if err := dynamic.Validate(); err != nil {
		t.Fatal(err)
	}
	dynamic.setupIndicators()

	if dynamic.Supertrend != nil {
		t.Fatal("did not expect static supertrend when adaptiveMultiplier is present")
	}
	if dynamic.DynamicSupertrend == nil {
		t.Fatal("expected dynamic supertrend")
	}
}

func TestAdaptiveMultiplierFromATR(t *testing.T) {
	atrValues := floats.Slice{1, 2, 3, 4, 5}

	if got, want := adaptiveMultiplierFromATR(atrValues, 5, 2, 4, adaptiveMultiplierPolarityHighVolHigh), 4.0; got != want {
		t.Fatalf("highVolHigh multiplier = %v, want %v", got, want)
	}
	if got, want := adaptiveMultiplierFromATR(atrValues, 5, 2, 4, adaptiveMultiplierPolarityHighVolLow), 2.0; got != want {
		t.Fatalf("highVolLow multiplier = %v, want %v", got, want)
	}

	lowCurrentATRValues := floats.Slice{5, 4, 3, 2, 1}
	if got, want := adaptiveMultiplierFromATR(lowCurrentATRValues, 5, 2, 4, adaptiveMultiplierPolarityHighVolHigh), 2.0; got != want {
		t.Fatalf("low current ATR multiplier = %v, want %v", got, want)
	}
}
