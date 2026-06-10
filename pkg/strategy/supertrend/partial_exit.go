package supertrend

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/indicator"
	"github.com/c9s/bbgo/pkg/types"
)

// PartialExit optionally closes part of a position after price reaches an ATR-based target from
// the entry bar. A nil *PartialExit is a no-op.
//
// Backtest-validated only. Live use would additionally require:
//   - arming on entry FILL confirmation rather than order submission (entry price/time may differ);
//   - coordination between the partial close order and later full-close paths (stop/reversal) to
//     avoid over-closing while the partial order is still unfilled;
//   - retry semantics: done is intentionally set even when the close submission fails, to avoid
//     duplicate close orders on subsequent bars.
type PartialExit struct {
	// AtrMultiple is the target distance in entry ATR multiples.
	AtrMultiple float64 `json:"atrMultiple"`
	// LockRatio is the fraction of the open position to close when the target is reached.
	LockRatio fixedpoint.Value `json:"lockRatio"`
	// AtrWindow is the ATR window used to snapshot entry volatility.
	AtrWindow int `json:"atrWindow"`

	atr        *indicator.ATR
	atrEndTime time.Time

	entryPrice   fixedpoint.Value
	entryATR     float64
	entryEndTime time.Time
	done         bool
	armed        bool
}

// Validate validates and applies defaults. A nil receiver is valid (disabled).
func (pe *PartialExit) Validate() error {
	if pe == nil {
		return nil
	}
	if pe.AtrMultiple == 0 {
		pe.AtrMultiple = 1.0
	}
	if pe.LockRatio.IsZero() {
		pe.LockRatio = fixedpoint.NewFromFloat(0.5)
	}
	if pe.AtrWindow == 0 {
		pe.AtrWindow = 14
	}
	if pe.AtrMultiple <= 0 {
		return errors.New("atrMultiple must be > 0")
	}
	if pe.LockRatio.Compare(fixedpoint.Zero) <= 0 || pe.LockRatio.Compare(fixedpoint.One) >= 0 {
		return errors.New("lockRatio must be > 0 and < 1")
	}
	if pe.AtrWindow < 1 {
		return errors.New("atrWindow must be >= 1")
	}
	return nil
}

// setupIndicators initializes the ATR backing the partial-exit target. A nil receiver is a no-op.
func (pe *PartialExit) setupIndicators(s *Strategy, kLineStore *types.MarketDataStore) {
	if pe == nil {
		return
	}
	if pe.AtrMultiple == 0 {
		pe.AtrMultiple = 1.0
	}
	if pe.LockRatio.IsZero() {
		pe.LockRatio = fixedpoint.NewFromFloat(0.5)
	}
	if pe.AtrWindow == 0 {
		pe.AtrWindow = 14
	}
	pe.atr = &indicator.ATR{IntervalWindow: types.IntervalWindow{Interval: s.Interval, Window: pe.AtrWindow}}
	// ATR.PushK has no explicit end-time guard here, so wrap it to avoid double-counting the same
	// kline across preload and live updates.
	s.session.MarketDataStream.OnKLineClosed(types.KLineWith(s.Symbol, s.Interval, pe.pushK))
	if klines, ok := kLineStore.KLinesOfInterval(s.Interval); ok {
		for i := range *klines {
			pe.pushK((*klines)[i])
		}
	}
}

// pushK feeds a kline to ATR, skipping any kline not strictly newer than the last one processed.
func (pe *PartialExit) pushK(k types.KLine) {
	if pe == nil || pe.atr == nil {
		return
	}
	end := k.EndTime.Time()
	if !pe.atrEndTime.IsZero() && !end.After(pe.atrEndTime) {
		return
	}
	pe.atr.PushK(k)
	pe.atrEndTime = end
}

// Reset clears per-episode state. A nil receiver is a no-op.
func (pe *PartialExit) Reset() {
	if pe == nil {
		return
	}
	pe.entryPrice = fixedpoint.Zero
	pe.entryATR = 0
	pe.entryEndTime = time.Time{}
	pe.done = false
	pe.armed = false
}

// Arm snapshots the entry close price and current ATR. A nil receiver is a no-op.
func (pe *PartialExit) Arm(kline types.KLine, entryPrice fixedpoint.Value) {
	if pe == nil {
		return
	}
	pe.entryPrice = entryPrice
	pe.entryATR = pe.lastATR()
	pe.entryEndTime = kline.EndTime.Time()
	pe.done = false
	pe.armed = true
}

func (pe *PartialExit) lastATR() float64 {
	if pe == nil || pe.atr == nil {
		return 0
	}
	return pe.atr.Last(0)
}

// CloseIfTriggered closes LockRatio of the current position when the entry-ATR target is reached.
// A nil receiver is a no-op.
func (pe *PartialExit) CloseIfTriggered(ctx context.Context, s *Strategy, kline types.KLine, closePrice fixedpoint.Value, fullClosedThisBar bool) {
	if pe == nil || s == nil || s.Position == nil {
		return
	}
	if fullClosedThisBar || !pe.armed || pe.done {
		return
	}
	if kline.EndTime.Time().Equal(pe.entryEndTime) {
		return
	}

	base := s.Position.GetBase()
	if s.Market.IsDustQuantity(base.Abs(), closePrice) {
		return
	}
	if !shouldTriggerPartialExit(kline, base, pe.entryPrice, pe.entryATR, pe.AtrMultiple) {
		return
	}

	quantity := base.Abs().Mul(pe.LockRatio)
	if s.Market.IsDustQuantity(quantity, closePrice) {
		pe.done = true
		return
	}

	bbgo.Notify("%s partial exit triggered; closing %v of position", s.Symbol, pe.LockRatio)
	if err := s.ClosePosition(ctx, pe.LockRatio, "partialExit"); err != nil {
		log.WithError(err).Errorf("can not place %s partial exit order", s.Symbol)
		bbgo.Notify("can not place %s partial exit order", s.Symbol)
	}
	pe.done = true
}

// shouldTriggerPartialExit is the pure target decision for a long/short position.
func shouldTriggerPartialExit(
	kline types.KLine, base fixedpoint.Value, entryPrice fixedpoint.Value, entryATR float64, atrMultiple float64,
) bool {
	if base.IsZero() || entryATR <= 0 || atrMultiple <= 0 {
		return false
	}
	distance := fixedpoint.NewFromFloat(entryATR * atrMultiple)
	if base.Sign() > 0 {
		return kline.GetHigh().Compare(entryPrice.Add(distance)) >= 0
	}
	return kline.GetLow().Compare(entryPrice.Sub(distance)) <= 0
}
