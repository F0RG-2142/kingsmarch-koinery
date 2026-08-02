package main

import (
	"database/sql"
	"errors"
	"time"
)

const (
	longWindow  = 24 * time.Hour
	shortWindow = 3 * time.Hour
	minHistory  = 48 * time.Hour
)

var errNotEnoughHistory = errors.New("not enough history yet (<48h)")

// recommendation holds the computed buy/sell targets for one currency.
type recommendation struct {
	Name       string
	Display    string
	Current    float64
	BuyTarget  float64
	SellTarget float64
}

// analyze computes buy/sell targets for every currency using volume-weighted
// stats and the Volume-Weighted Momentum Skew (VMS), all inside DuckDB.
func analyze(db *sql.DB, now time.Time) ([]recommendation, error) {
	// Cold-start guard: we need at least minHistory of accumulated snapshots
	var minTs sql.NullTime
	if err := db.QueryRow(`SELECT MIN(ts) FROM price_snapshots`).Scan(&minTs); err != nil {
		return nil, err
	}
	if !minTs.Valid || now.Sub(minTs.Time) < minHistory {
		return nil, errNotEnoughHistory
	}

	const query = `
WITH
long_w AS (
    SELECT name, price, quantity
    FROM price_snapshots
    WHERE ts >= ? - INTERVAL '24 hours'
),
short_w AS (
    SELECT name, price, quantity
    FROM price_snapshots
    WHERE ts >= ? - INTERVAL '3 hours'
),
long_stats AS (
    SELECT name,
           SUM(price*quantity)/NULLIF(SUM(quantity),0) AS mu
    FROM long_w GROUP BY name
),
sigma AS (
    SELECT s.name, s.mu,
           SQRT(SUM(lw.quantity * POW(lw.price - s.mu, 2)) / NULLIF(SUM(lw.quantity),0)) AS sigma
    FROM long_w lw JOIN long_stats s USING (name)
    GROUP BY s.name, s.mu
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
    SELECT c.name, c.text, c.cur, c.cur_quantity, sig.mu, sig.sigma, ss.mu_short
    FROM cur c
    JOIN sigma sig USING (name)
    JOIN short_stats ss USING (name)
)
SELECT name, text,
       cur,
       mu - 2.5*sigma + LEAST(GREATEST((mu_short - mu)/NULLIF(mu,0), -1), 1)*sigma AS buy_target,
       mu + 1.5*sigma + LEAST(GREATEST((mu_short - mu)/NULLIF(mu,0), -1), 1)*sigma
           + CASE WHEN cur_quantity >= 500 THEN 0.5*sigma ELSE 0 END AS sell_target
FROM combined
ORDER BY name`

	rows, err := db.Query(query, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []recommendation
	for rows.Next() {
		var r recommendation
		if err := rows.Scan(&r.Name, &r.Display, &r.Current, &r.BuyTarget, &r.SellTarget); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}
