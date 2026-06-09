package supertrend

import (
	"github.com/pkg/errors"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
)

// EntryFilters is an optional set of veto-only entry-quality filters layered on top of the
// supertrend entry signal. A filter can only reject (veto) an entry that the base signal would
// otherwise take; it never creates a new entry. A nil *EntryFilters is a no-op, so existing
// configs that omit `entryFilters` behave exactly as before.
type EntryFilters struct {
	// HTFTrend vetoes entries that disagree with the higher-timeframe trend direction.
	HTFTrend *HTFTrendFilter `json:"htfTrend,omitempty"`
}

// Validate validates the filter configuration. A nil receiver is valid (filters disabled).
func (ef *EntryFilters) Validate() error {
	if ef == nil {
		return nil
	}
	if ef.HTFTrend != nil {
		if err := ef.HTFTrend.validate(); err != nil {
			return errors.Wrap(err, "htfTrend filter")
		}
	}
	return nil
}

// Subscribe subscribes the klines required by the enabled filters. A nil receiver is a no-op.
func (ef *EntryFilters) Subscribe(session *bbgo.ExchangeSession, symbol string) {
	if ef == nil {
		return
	}
	if ef.HTFTrend != nil {
		session.Subscribe(types.KLineChannel, symbol, types.SubscribeOptions{Interval: ef.HTFTrend.Interval})
	}
}

// setupIndicators initializes the indicators backing the enabled filters. A nil receiver is a no-op.
func (ef *EntryFilters) setupIndicators(s *Strategy, kLineStore *types.MarketDataStore) {
	if ef == nil {
		return
	}
	if ef.HTFTrend != nil {
		ef.HTFTrend.setup(s, kLineStore)
	}
}

// FilterOpenSide applies the enabled filters to the proposed entry side and returns the side that
// should actually be opened. It returns the input side unchanged when no filter is active or the
// side is not a real entry (empty). A vetoed entry returns an empty SideType. A nil receiver
// returns the side unchanged.
func (ef *EntryFilters) FilterOpenSide(side types.SideType, kline types.KLine) types.SideType {
	if ef == nil {
		return side
	}
	if side != types.SideTypeBuy && side != types.SideTypeSell {
		return side
	}

	if ef.HTFTrend != nil && !ef.HTFTrend.allows(side) {
		ef.HTFTrend.vetoCount++
		log.Debugf("entryFilters: htfTrend vetoed %s entry (htfTrend=%v, vetoCount=%d)", side, ef.HTFTrend.signal(), ef.HTFTrend.vetoCount)
		return types.SideType("")
	}

	return side
}

// HTFTrendFilter vetoes entries that disagree with the higher-timeframe (HTF) trend direction,
// measured by the sign of a linear-regression slope on the HTF interval. In an HTF uptrend only
// long entries are allowed; in an HTF downtrend only short entries are allowed. While the HTF
// indicator is not yet warmed up (slope == 0 / no signal) the filter does not veto, to avoid
// cold-start false rejections.
type HTFTrendFilter struct {
	// Interval is the higher timeframe used to measure the trend. It should be >= the strategy
	// interval (typically an integer multiple, e.g. strategy 15m -> HTF 1h/4h).
	Interval types.Interval `json:"interval"`
	// Window is the linear-regression window on the HTF interval.
	Window int `json:"window"`

	linReg    *LinReg
	vetoCount int
}

func (f *HTFTrendFilter) validate() error {
	if len(f.Interval) == 0 {
		return errors.New("interval is required")
	}
	if f.Window < 0 {
		return errors.New("window must be >= 0")
	}
	return nil
}

func (f *HTFTrendFilter) setup(s *Strategy, kLineStore *types.MarketDataStore) {
	if f.Window == 0 {
		f.Window = 20
	}
	f.linReg = &LinReg{IntervalWindow: types.IntervalWindow{Interval: f.Interval, Window: f.Window}}
	f.linReg.BindK(s.session.MarketDataStream, s.Symbol, f.Interval)
	if klines, ok := kLineStore.KLinesOfInterval(f.Interval); ok {
		f.linReg.LoadK((*klines)[0:])
	}
}

// signal returns the persistent HTF trend direction (Up/Down) from the linreg slope sign, or
// DirectionNone when not yet warmed up.
func (f *HTFTrendFilter) signal() types.Direction {
	if f.linReg == nil {
		return types.DirectionNone
	}
	return f.linReg.GetSignal()
}

// allows reports whether the given entry side is permitted under the current HTF trend.
func (f *HTFTrendFilter) allows(side types.SideType) bool {
	return htfTrendAllows(f.signal(), side)
}

// htfTrendAllows is the pure decision function: an entry is vetoed only when it clearly opposes a
// known HTF trend. An unknown trend (DirectionNone) never vetoes.
func htfTrendAllows(trend types.Direction, side types.SideType) bool {
	switch trend {
	case types.DirectionUp:
		return side != types.SideTypeSell
	case types.DirectionDown:
		return side != types.SideTypeBuy
	default:
		return true
	}
}
