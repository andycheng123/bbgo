package indicator

import (
	"math"
	"time"

	"github.com/c9s/bbgo/pkg/datatype/floats"
	"github.com/c9s/bbgo/pkg/types"
)

// SupertrendDynamic is a Supertrend variant whose ATR multiplier can change per bar.
type SupertrendDynamic struct {
	types.SeriesBase
	types.IntervalWindow
	ATRMultiplier float64 `json:"atrMultiplier"`

	AverageTrueRange *ATR

	trendPrices    floats.Slice
	supportLine    floats.Slice
	resistanceLine floats.Slice

	closePrice             float64
	previousClosePrice     float64
	uptrendPrice           float64
	previousUptrendPrice   float64
	downtrendPrice         float64
	previousDowntrendPrice float64

	trend         types.Direction
	previousTrend types.Direction
	tradeSignal   types.Direction

	EndTime         time.Time
	UpdateCallbacks []func(value float64)
}

func (inc *SupertrendDynamic) SetMultiplier(multiplier float64) {
	inc.ATRMultiplier = multiplier
}

func (inc *SupertrendDynamic) Last(i int) float64 {
	return inc.trendPrices.Last(i)
}

func (inc *SupertrendDynamic) Index(i int) float64 {
	return inc.Last(i)
}

func (inc *SupertrendDynamic) Length() int {
	return len(inc.trendPrices)
}

func (inc *SupertrendDynamic) Update(highPrice, lowPrice, closePrice float64) {
	if inc.Window <= 0 {
		panic("window must be greater than 0")
	}

	if inc.AverageTrueRange == nil {
		inc.SeriesBase.Series = inc
	}

	// Start with DirectionUp
	if inc.trend != types.DirectionUp && inc.trend != types.DirectionDown {
		inc.trend = types.DirectionUp
	}

	// Update ATR
	inc.AverageTrueRange.Update(highPrice, lowPrice, closePrice)

	// Update last prices
	inc.previousUptrendPrice = inc.uptrendPrice
	inc.previousDowntrendPrice = inc.downtrendPrice
	inc.previousClosePrice = inc.closePrice
	inc.previousTrend = inc.trend

	inc.closePrice = closePrice

	src := (highPrice + lowPrice) / 2
	multiplier := inc.ATRMultiplier

	// Update uptrend
	inc.uptrendPrice = src - inc.AverageTrueRange.Last(0)*multiplier
	if inc.previousClosePrice > inc.previousUptrendPrice {
		inc.uptrendPrice = math.Max(inc.uptrendPrice, inc.previousUptrendPrice)
	}

	// Update downtrend
	inc.downtrendPrice = src + inc.AverageTrueRange.Last(0)*multiplier
	if inc.previousClosePrice < inc.previousDowntrendPrice {
		inc.downtrendPrice = math.Min(inc.downtrendPrice, inc.previousDowntrendPrice)
	}

	// Update trend
	if inc.previousTrend == types.DirectionUp && inc.closePrice < inc.previousUptrendPrice {
		inc.trend = types.DirectionDown
	} else if inc.previousTrend == types.DirectionDown && inc.closePrice > inc.previousDowntrendPrice {
		inc.trend = types.DirectionUp
	} else {
		inc.trend = inc.previousTrend
	}

	// Update signal
	if inc.AverageTrueRange.Last(0) <= 0 {
		inc.tradeSignal = types.DirectionNone
	} else if inc.trend == types.DirectionUp && inc.previousTrend == types.DirectionDown {
		inc.tradeSignal = types.DirectionUp
	} else if inc.trend == types.DirectionDown && inc.previousTrend == types.DirectionUp {
		inc.tradeSignal = types.DirectionDown
	} else {
		inc.tradeSignal = types.DirectionNone
	}

	// Update trend price
	if inc.trend == types.DirectionDown {
		inc.trendPrices.Push(inc.downtrendPrice)
	} else {
		inc.trendPrices.Push(inc.uptrendPrice)
	}

	// Save the trend lines
	inc.supportLine.Push(inc.uptrendPrice)
	inc.resistanceLine.Push(inc.downtrendPrice)

	logst.Debugf("Update dynamic supertrend result: closePrice: %v, uptrendPrice: %v, downtrendPrice: %v, trend: %v,"+
		" tradeSignal: %v, AverageTrueRange.Last(): %v, ATRMultiplier: %v", inc.closePrice, inc.uptrendPrice, inc.downtrendPrice,
		inc.trend, inc.tradeSignal, inc.AverageTrueRange.Last(0), inc.ATRMultiplier)
}

func (inc *SupertrendDynamic) GetSignal() types.Direction {
	return inc.tradeSignal
}

// Direction return the current trend.
func (inc *SupertrendDynamic) Direction() types.Direction {
	return inc.trend
}

// LastSupertrendSupport return the current supertrend support.
func (inc *SupertrendDynamic) LastSupertrendSupport() float64 {
	return inc.supportLine.Last(0)
}

// LastSupertrendResistance return the current supertrend resistance.
func (inc *SupertrendDynamic) LastSupertrendResistance() float64 {
	return inc.resistanceLine.Last(0)
}

var _ types.SeriesExtend = &SupertrendDynamic{}

func (inc *SupertrendDynamic) PushK(k types.KLine) {
	if inc.EndTime != zeroTime && k.EndTime.Before(inc.EndTime) {
		return
	}

	inc.Update(k.GetHigh().Float64(), k.GetLow().Float64(), k.GetClose().Float64())
	inc.EndTime = k.EndTime.Time()
	inc.EmitUpdate(inc.Last(0))
}

func (inc *SupertrendDynamic) BindK(target KLineClosedEmitter, symbol string, interval types.Interval) {
	target.OnKLineClosed(types.KLineWith(symbol, interval, inc.PushK))
}

func (inc *SupertrendDynamic) LoadK(allKLines []types.KLine) {
	for _, k := range allKLines {
		inc.PushK(k)
	}
}

func (inc *SupertrendDynamic) CalculateAndUpdate(kLines []types.KLine) {
	for _, k := range kLines {
		if inc.EndTime != zeroTime && !k.EndTime.After(inc.EndTime) {
			continue
		}

		inc.PushK(k)
	}

	inc.EmitUpdate(inc.Last(0))
	inc.EndTime = kLines[len(kLines)-1].EndTime.Time()
}

func (inc *SupertrendDynamic) handleKLineWindowUpdate(interval types.Interval, window types.KLineWindow) {
	if inc.Interval != interval {
		return
	}

	inc.CalculateAndUpdate(window)
}

func (inc *SupertrendDynamic) Bind(updater KLineWindowUpdater) {
	updater.OnKLineWindowUpdate(inc.handleKLineWindowUpdate)
}

func (inc *SupertrendDynamic) OnUpdate(cb func(value float64)) {
	inc.UpdateCallbacks = append(inc.UpdateCallbacks, cb)
}

func (inc *SupertrendDynamic) EmitUpdate(value float64) {
	for _, cb := range inc.UpdateCallbacks {
		cb(value)
	}
}
