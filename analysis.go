package main

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	longWindow  = 24 * time.Hour
	shortWindow = 3 * time.Hour
	minHistory  = 48 * time.Hour

	// halfLifeWindow is how far back the OU mean-reversion fit looks. Mean
	// reversion speed is a structural property, so it is measured over a longer
	// span than the (24h) band window, which only prices the current trade.
	halfLifeWindow = 7 * 24 * time.Hour

	// OU mean-reversion filter. A currency only gets a recommendation if it
	// actually reverts to its mean (half-life within these bounds). Half-life
	// outside this band means it either whipsaws (too fast) or trends (too
	// slow to capture in a few days).
	minHalfLife = 1 * time.Hour
	maxHalfLife = 48 * time.Hour

	// bulkOrderSize is the order size (current hourly quantity) that earns the
	// bulk premium.
	bulkOrderSize = 500

	// sigmaOverMuMax is a loosened sanity guard on the coefficient of
	// variation. It is deliberately wide: the half-life filter is the real
	// discriminator for tradability, not this ratio.
	sigmaOverMuMax = 0.8
)

var errNotEnoughHistory = errors.New("not enough history yet (<48h)")

// recommendation holds the computed buy/sell targets for one currency.
type recommendation struct {
	Name       string
	Display    string
	Current    float64
	BuyTarget  float64
	SellTarget float64
	BulkSell   float64
	Quantity   int64
	HalfLife   time.Duration
}

// analyze computes buy/sell targets for every currency using volume-weighted
// stats and the Volume-Weighted Momentum Skew (VMS), then drops currencies
// that do not mean-revert (OU half-life outside [minHalfLife, maxHalfLife]).
func analyze(db *sql.DB, now time.Time) ([]recommendation, error) {
	// Cold-start guard: we need at least minHistory of accumulated snapshots.
	var minTs sql.NullTime
	if err := db.QueryRow(`SELECT MIN(ts) FROM price_snapshots`).Scan(&minTs); err != nil {
		return nil, err
	}
	if !minTs.Valid || now.Sub(minTs.Time) < minHistory {
		return nil, errNotEnoughHistory
	}

	// The window lengths are the single source of truth here; they are injected
	// into the SQL instead of being duplicated as string literals.
	longH := int(longWindow / time.Hour)
	shortH := int(shortWindow / time.Hour)

	query := fmt.Sprintf(`
WITH
long_w AS (
    SELECT name, price, quantity
    FROM price_snapshots
    WHERE ts >= ? - INTERVAL '%d hours'
),
short_w AS (
    SELECT name, price, quantity
    FROM price_snapshots
    WHERE ts >= ? - INTERVAL '%d hours'
),
long_stats AS (
    SELECT name,
           SUM(price*quantity)/NULLIF(SUM(quantity),0) AS mu,
           SUM(quantity) AS total_volume,
           COUNT(*) AS n_snapshots
    FROM long_w GROUP BY name
),
sigma AS (
    SELECT s.name, s.mu, s.total_volume, s.n_snapshots,
           SQRT(SUM(lw.quantity * POW(lw.price - s.mu, 2)) / NULLIF(SUM(lw.quantity),0)) AS sigma
    FROM long_w lw JOIN long_stats s USING (name)
    GROUP BY s.name, s.mu, s.total_volume, s.n_snapshots
),
short_stats AS (
    SELECT name, SUM(price*quantity)/NULLIF(SUM(quantity),0) AS mu_short
    FROM short_w GROUP BY name
),
latest AS (
    SELECT name, text, price, quantity,
           ROW_NUMBER() OVER (PARTITION BY name ORDER BY ts DESC) AS rn
    FROM price_snapshots
),
cur AS (
    SELECT name, text, price AS cur, quantity AS cur_quantity
    FROM latest WHERE rn = 1
),
combined AS (
    SELECT c.name, c.text, c.cur, c.cur_quantity, sig.mu, sig.sigma, ss.mu_short,
           sig.total_volume, sig.n_snapshots,
           sig.sigma / NULLIF(sig.mu, 0) AS sigma_over_mu,
           TANH((ss.mu_short - sig.mu) / NULLIF(sig.sigma, 0)) AS vms
    FROM cur c
    JOIN sigma sig USING (name)
    JOIN short_stats ss USING (name)
)
SELECT name, text,
       cur,
       cur_quantity,
       mu - 2.5*sigma + vms*1.5*sigma AS buy_target,
       mu + 1.5*sigma + vms*1.5*sigma AS sell_target,
       mu + 1.5*sigma + vms*1.5*sigma
           + CASE WHEN cur_quantity >= %d THEN 0.5*sigma ELSE 0 END AS bulk_sell
FROM combined
WHERE n_snapshots >= 6
  AND total_volume >= 50
  AND sigma_over_mu <= %v
  AND (mu - 2.5*sigma + vms*1.5*sigma) > 0
ORDER BY name`, longH, shortH, bulkOrderSize, sigmaOverMuMax)

	rows, err := db.Query(query, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []recommendation
	for rows.Next() {
		var r recommendation
		if err := rows.Scan(&r.Name, &r.Display, &r.Current, &r.Quantity, &r.BuyTarget, &r.SellTarget, &r.BulkSell); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Mean-reversion filter: drop currencies whose price series does not revert
	// to its mean within a useful holding window (OU half-life).
	series, err := loadSeries(db, now.Add(-halfLifeWindow))
	if err != nil {
		return nil, err
	}
	out := make([]recommendation, 0, len(recs))
	for _, r := range recs {
		prices, ok := series[r.Name]
		if !ok {
			continue
		}
		hl, reverter := ouHalfLife(prices, time.Hour)
		if !reverter || hl < minHalfLife || hl > maxHalfLife {
			continue
		}
		r.HalfLife = hl
		out = append(out, r)
	}
	return out, nil
}

// loadSeries returns, per currency, the ordered hourly price series over the
// given window (ordered oldest to newest).
func loadSeries(db *sql.DB, since time.Time) (map[string][]float64, error) {
	rows, err := db.Query(`SELECT name, price FROM price_snapshots WHERE ts >= ? ORDER BY name, ts`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	series := make(map[string][]float64)
	for rows.Next() {
		var name string
		var price float64
		if err := rows.Scan(&name, &price); err != nil {
			return nil, err
		}
		series[name] = append(series[name], price)
	}
	return series, rows.Err()
}

// ouHalfLife estimates the mean-reversion half-life of a price series via the
// AR(1)-style regression ΔP_t = α + γ·P_{t-1} + ε. θ = -γ is the reversion
// speed, so the half-life is ln(2)/θ. Returns ok=false when the series does
// not mean-revert (γ ≥ 0) or there is too little data to fit.
func ouHalfLife(prices []float64, interval time.Duration) (time.Duration, bool) {
	n := len(prices)
	if n < 4 {
		return 0, false
	}
	// OLS slope of ΔP on lagged P: γ = Cov(P_{t-1}, ΔP) / Var(P_{t-1}).
	var sx, sy, sxx, sxy float64
	for i := 1; i < n; i++ {
		x := prices[i-1]
		y := prices[i] - prices[i-1]
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	m := float64(n - 1)
	meanX := sx / m
	meanY := sy / m
	var num, den float64
	for i := 1; i < n; i++ {
		x := prices[i-1]
		y := prices[i] - prices[i-1]
		num += (x - meanX) * (y - meanY)
		den += (x - meanX) * (x - meanX)
	}
	if den == 0 {
		return 0, false
	}
	gamma := num / den
	theta := -gamma
	if theta <= 0 || math.IsInf(theta, 0) {
		return 0, false
	}
	hl := math.Ln2 / theta // in steps of `interval`
	if math.IsInf(hl, 0) || hl < 0 {
		return 0, false
	}
	return time.Duration(hl * float64(interval)), true
}
