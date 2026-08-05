package main

import (
	"fmt"
	"math"
	"net/http"

	"kingsmarch-koinery/apiCalls"
)

// Backtest config. PriceLogs is per-day, so the rolling window is in days.
const (
	backtestWindow   = 3 // rolling lookback (in daily price points)
	backtestMinPoints = 6
)

// buy/sell multiplier grid searched per currency.
var (
	backtestKs = []float64{1.5, 2.0, 2.5, 3.0}
	backtestJs = []float64{1.0, 1.5, 2.0}
)

// btResult is the outcome of one (k, j) configuration for one currency.
type btResult struct {
	Name        string
	K, J        float64
	Trades      int
	WinRate     float64
	AvgReturn   float64 // fractional return per closed trade
	TotalReturn float64 // compounded fractional return across all trades
	MaxDD       float64 // fractional max drawdown on the compounded curve
}

// runBacktest fetches current PriceLogs and reports the best (buy, sell)
// band multipliers per currency, so the σ multipliers are data-driven rather
// than guessed.
func runBacktest(client *http.Client, league string) {
	data, err := apiCalls.CallApi(client, league, 1)
	if err != nil {
		fmt.Printf("backtest: %v\n", err)
		return
	}

	fmt.Printf("%-22s %5s %5s %6s %7s %9s %10s %8s\n",
		"name", "k", "j", "trades", "win%", "avg%", "total%", "maxDD%")

	qualified := 0
	for _, it := range data.Items {
		prices := make([]float64, 0, len(it.PriceLogs))
		for _, pl := range it.PriceLogs {
			prices = append(prices, pl.Price)
		}
		// PriceLogs is newest-first; reverse so the simulation runs oldest→newest.
		reverse(prices)

		if len(prices) < backtestMinPoints {
			continue
		}
		qualified++

		best := bestParams(prices, backtestWindow)
		best.Name = it.Name
		if best.Trades == 0 {
			fmt.Printf("%-22s %5s %5s %6s %7s %9s %10s %8s\n",
				it.Name, "-", "-", "0", "-", "-", "-", "-")
			continue
		}
		fmt.Printf("%-22s %5.1f %5.1f %6d %6.0f%% %8.2f%% %9.2f%% %7.2f%%\n",
			it.Name, best.K, best.J, best.Trades,
			best.WinRate*100, best.AvgReturn*100, best.TotalReturn*100, best.MaxDD*100)
	}

	fmt.Printf("\n%d currencies had >= %d price points to backtest.\n", qualified, backtestMinPoints)
	fmt.Println("PriceLogs is per-day and grows over time; rerun -backtest as history accumulates to tighten the multipliers.")
}

// bestParams grid-searches (k, j) and returns the configuration with the
// best risk-adjusted score, or a zero result when no (k, j) produced trades.
func bestParams(prices []float64, window int) btResult {
	var best btResult
	for _, k := range backtestKs {
		for _, j := range backtestJs {
			rets, dd := bandSim(prices, window, k, j)
			res := summarize(k, j, rets, dd)
			if res.Trades > 0 && score(res) > score(best) {
				best = res
			}
		}
	}
	return best
}

// bandSim runs the asymmetric mean-reversion strategy over a price series:
// enter long when price dips below (rolling mean − k·σ); exit when it rises
// back above (rolling mean + j·σ). Returns the per-trade returns and the max
// drawdown of the compounded curve.
func bandSim(prices []float64, window int, k, j float64) (rets []float64, maxDD float64) {
	n := len(prices)
	equity, peak := 1.0, 1.0
	for i := window; i < n; {
		mu, sd := meanStd(prices[i-window : i])
		if sd == 0 {
			i++
			continue
		}
		p := prices[i]
		if p < mu-k*sd {
			// Enter long.
			buy := p
			j2 := i + 1
			for ; j2 < n; j2++ {
				mu2, sd2 := meanStd(prices[j2-window : j2])
				if sd2 == 0 {
					continue
				}
				if prices[j2] > mu2+j*sd2 {
					break
				}
			}
			// Exit at the first bar that crosses the sell band, or at series end.
			exit := prices[n-1]
			if j2 < n {
				exit = prices[j2]
			}
			ret := (exit - buy) / buy
			rets = append(rets, ret)
			equity *= 1 + ret
			if equity > peak {
				peak = equity
			}
			if dd := (peak - equity) / peak; dd > maxDD {
				maxDD = dd
			}
			i = j2 + 1
		} else {
			i++
		}
	}
	return rets, maxDD
}

func summarize(k, j float64, rets []float64, maxDD float64) btResult {
	res := btResult{K: k, J: j, Trades: len(rets), MaxDD: maxDD}
	if len(rets) == 0 {
		return res
	}
	wins, sum := 0, 0.0
	for _, r := range rets {
		sum += r
		if r > 0 {
			wins++
		}
	}
	res.WinRate = float64(wins) / float64(len(rets))
	res.AvgReturn = sum / float64(len(rets))
	res.TotalReturn = 1
	for _, r := range rets {
		res.TotalReturn *= 1 + r
	}
	res.TotalReturn -= 1
	return res
}

// score ranks a result: reward per-trade edge, win rate, and number of trades.
func score(r btResult) float64 {
	if r.Trades == 0 {
		return 0
	}
	return r.AvgReturn * r.WinRate * math.Sqrt(float64(r.Trades))
}

func meanStd(v []float64) (mean, sd float64) {
	if len(v) == 0 {
		return 0, 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	mean = s / float64(len(v))
	var ss float64
	for _, x := range v {
		d := x - mean
		ss += d * d
	}
	sd = math.Sqrt(ss / float64(len(v)))
	return mean, sd
}

func reverse(v []float64) {
	for i, j := 0, len(v)-1; i < j; i, j = i+1, j-1 {
		v[i], v[j] = v[j], v[i]
	}
}
