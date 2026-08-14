# PoE2 Stat-Arb & Volatility Trading
*Statistical Arbitrage and Volatility Trading in Path of Exile 2*

<br>The project is live and hosted, if you want access, you can probably pop me an email.<br>

Here is a screenshot of what the analysis message looks like that gets sent every 4 hours:

<img width="1280" height="925" alt="image" src="https://github.com/user-attachments/assets/67b54b2b-4140-49a4-acbb-d014a7436b99" />

I'm using the [poe2scout API](https://api.poe2scout.com/swagger/index.html)

Using currency by category to track currency prices. Note: the endpoint ignores `pageNumber`/`pageSize` and always returns the same single page of items (in my experience?).

The API exposes **per-day `PriceLogs`** for each currency, but not per-hour history. However, the "current price" and "current volume" fields update every hour, so the design is **stateful**: we save an API snapshot to DuckDB every hour and run the analysis on the accumulated history. The daily `PriceLogs` are parsed too and feed the **backtest** (see below).


---

## The Pipeline

> **Pull API Data** (every 60 mins) ➔ **Run Math™** (analyze promising currencies) ➔ **Discord Alerts** (send top 10 pairs every few hours) ➔ **Manual Execution** (automation is illegal, don't get banned)

---

## 3 Strategies to Maximize Currency

### 1. Volume-Weighted Asymmetric Mean Reversion
We calculate the rolling mean and standard deviation over a dynamic timeframe. Crucially, this is *weighted by trade volume* to filter out low-liquidity price fixing and market manipulation.

* Place a target **buy order** at the bottom band (e.g., 2.5 standard deviations).
* Place a target **sell order** at the top band (e.g., 1.5 standard deviations).
* Boom, profit.

**Volume-Weighted Mean:**

$$
\mu_{vw} = \frac{\sum_{i=1}^{n} P_i V_i}{\sum_{i=1}^{n} V_i}
$$

*(Where `P_i` is the price at a given tick, `V_i` is the volume traded at that tick, and `n` is the time window.)*

**Volume-Weighted Variance & Std Dev:**

$$
\sigma_{vw}^2 = \frac{\sum_{i=1}^{n} V_i (P_i - \mu_{vw})^2}{\sum_{i=1}^{n} V_i}
$$

$$
\sigma_{vw} = \sqrt{\sigma_{vw}^2}
$$

**Base Asymmetric Bands:**

$$
Buy_{base} = \mu_{vw} - 2.5\sigma_{vw}
$$

$$
Sell_{base} = \mu_{vw} + 1.5\sigma_{vw}
$$

### 2. Volume-Weighted Momentum Skew (VMS)
Since poe2scout doesn't expose a live order book, we cannot look at pending buyer/seller intent. Instead, we look at actual historical aggression—whether short-term price action is ripping or dipping relative to the long-term trend, weighted by volume.

We compare a short-term volume-weighted average ($\mu_{short}$, e.g., last 3 hours) against our baseline long-term average ($\mu_{vw}$, e.g., last 24 hours). To keep our skew bounded between $-1$ and $1$ (just like an OBI model), we run the deviation through a hyperbolic tangent ($\tanh$) function. **Crucially, the deviation is normalized by $\sigma_{vw}$**, so the skew is scale-free relative to the currency's actual price noise — a 3% move on a calm currency and a 3% move on a noisy one are treated differently.

* If short-term prices are surging above the trend (**VMS approaches 1**), buyers are aggressive. We shift our entire bracket upward so we don't get left behind by a breakout.
* If short-term prices are crashing below the trend (**VMS approaches -1**), sellers are dumping. We shift our bracket downward to catch the falling knife safely.

**The Momentum Skew:**

$$
VMS = \tanh \left( \frac{\mu_{short} - \mu_{vw}}{\sigma_{vw}} \right)
$$

*(Where `\mu_{short}` is the VWMA of a narrow window, `\mu_{vw}` is the long-term VWMA window, and `\sigma_{vw}` is the long-term standard deviation to scale the variance.)*

**The Skew Adjustment:**

$$
Skew = VMS \times 1.5
$$

*(Where 1.5 is a tuning constant.)*

> ***"But, like, what if something just keeps spiraling into oblivion?"***
>  
> If VMS locks at -1, our target buy price drops heavily, protecting us from buying into a dead economy until the velocity flattens out.

### 3. Bulk Premium
You know what sucks? Buying 100 Divine Orbs here, 50 there, and another 70 somewhere else. 

Since I'm making everyone's life easier by selling in bulk, you best believe I'm charging for it. If I sell a stack larger than 500 at a time, I'm adding another 0.5 standard deviations to the price.

**Bulk Premium:**

$$
Premium_{bulk} = 0.5\sigma_{vw} \quad \text{(if volume } \ge 500)
$$

$$
Premium_{bulk} = 0 \quad \text{(otherwise)}
$$

---

## 4. Mean Reversion Strength (OU Half-Life)

Mean reversion is a nice story until a currency just decides it doesn't want to revert and trends forever instead. So before we trust any band, we **fit an Ornstein-Uhlenbeck process** to the hourly price series and ask: *does this thing actually snap back to its mean, and how fast?*

We regress the price change on the lagged price level:

$$
\Delta P_t = \alpha + \gamma P_{t-1} + \varepsilon
$$

The reversion speed is $\theta = -\gamma$, and the **half-life** (how long it takes to close half the gap to the mean) is:

$$
t_{1/2} = \frac{\ln 2}{\theta}
$$

* If a currency's half-life is **too short** — it whipsaws faster than we can fill orders without eating the spread.
* If it's **too long** — it basically trends; our "buy the dip" is really "catch the falling knife" forever.
* If $\theta \le 0$ — **it doesn't revert at all.** Dead to us.

Only currencies with a half-life in a useful band (roughly **1h to 48h** on our hourly data) get a recommendation. This is the real tradability filter — it replaces guessing with a fitted, testable number.

> ***"Wait, doesn't the VMS thing already handle trends?"***
>  
> VMS tells us *where* to put the bracket this hour. The half-life tells us *whether this currency is even worth bracketing at all*. They're orthogonal — and you need both. A trending currency will happily keep firing "buy" signals that bleed you dry while it walks off a cliff. The half-life check stops it at the door.

In the alert image, the **Reversion** column shows the same half-life, formatted as an expected hold time: `9h` means "close half the gap in 9 hours," `1d 9h` means "overnight at the very least," and `-` means "we dropped this currency because it doesn't revert."

### How to read the alert (cheat sheet)
A non-technical user can read the image top-to-bottom like this:

| What you see | What it means | Trade implication |
|---|---|---|
| **Spread ≥ 10%** and **Reversion ≤ 4h** | Quick scalp | Post both brackets, expect to fill both within a session. |
| **Spread ≥ 10%** and **Reversion 4h–24h** | Patient trade | You're committing overnight. Spread should comfortably cover a round-trip fee (gold fees are TODO). |
| **Spread ≥ 10%** and **Reversion > 24h** | Deep mean-reversion bet | Question whether the league is still alive in a week before committing. |
| **Spread < 5%** | Fees will eat it. | Walk away. |
| **Reversion `-`** | Currency didn't mean-revert. | It was filtered out at the door. |
| **Bulk > Sell** and **order ≥ 500** | Bulk window is open. | Premium pricing only applies if you're actually unloading a stack. |

The numbers in every column are **fitted from history**, not guessed.

---

## The Final Formula

Bringing it all together, our dynamic target limits look like this:

**Target Buy:**

$$
Buy_{final} = \mu_{vw} - 2.5\sigma_{vw} + (VMS \times 1.5\sigma_{vw})
$$

**Target Sell:**

$$
Sell_{final} = \mu_{vw} + 1.5\sigma_{vw} + (VMS \times 1.5\sigma_{vw}) + Premium_{bulk}
$$

...and a currency only makes it into the alert at all if its OU half-life says it actually reverts.

---

## Backtesting (fits the multipliers, not guesses them)

The 2.5 / 1.5 / 0.5 sigma multipliers used to be vibes. Now they're *data*.

`-backtest` pulls each currency's daily `PriceLogs` and runs the asymmetric mean-reversion strategy over the history for a **grid of (buy k, sell j) multipliers**, e.g. k ∈ {1.5, 2.0, 2.5, 3.0} and j ∈ {1.0, 1.5, 2.0}. For every combination it simulates: **enter long when price dips below (rolling mean − kσ), exit when it returns above (rolling mean + jσ)**. It then reports the winning config per currency:

```
name                       k     j trades    win%      avg%     total%   maxDD%
perfect-regal-orb        1.5   1.0      1    100%     5.27%      5.27%    0.00%
gcp                      1.5   1.0      1    100%     9.96%      9.96%    0.00%
```

Because `PriceLogs` is **per-day** and currently only a week deep, most currencies don't have enough history to produce trades yet. Rerun `-backtest` as the log grows — the multipliers will tighten from "best guess" to "fitted to your league's actual behavior." Gross returns only, for now.

---

### Disclaimer / TODO: Gold fees
*This model does not yet incorporate in-game Gold fees (YET).* Fees are a **known, deferred TODO** — the backtest reports **gross** returns, and the buy/sell targets don't subtract a round-trip fee. Before trading anything for real, net the fee against the band edge.

Known per-item listing fees:

Chaos Orb: 160 Gold (Item ID: chaos)

Exalted Orb: 120 Gold (Item ID: exalted)

Divine Orb: 800 (Item ID: divine)

---

## Noise Filters

A currency is only included in the alert if it has enough data to produce a meaningful band. If a row fails any of these safeguards, it is silently dropped (so a real opportunity is never hidden behind a cap — only noise is filtered):

- **Sample size:** `COUNT(*) >= 6` snapshots in the 24h window. Below this, the volume-weighted σ is meaningless.
- **Volume denominator:** `SUM(quantity) >= 50`. Below this, the volume-weighted mean is dominated by a single trade.
- **Variance sanity:** `σ / μ <= 0.8`. Deliberately wide — a loud currency isn't necessarily untradeable. The OU half-life check (not this ratio) is the real discriminator for whether we can bracket it.
- **Mean reversion:** OU half-life within **1h–48h**. Outside that band, or if it doesn't revert at all, the currency is dropped.
- **Sanity check:** `buy_target > 0` (the formula can't produce a negative buy).

**Bulksell** is shown as a separate column in the Discord output — visible alongside the regular sell when the current quantity ≥ 500.

---

## Discord Output

The analysis is rendered as a styled PNG image (color-coded columns, full table, no character limit) and attached to the webhook via `multipart/form-data`. The image shows one row per currency, sorted by spread, with columns: **Currency, Current, Buy, Sell, Bulk, Reversion (half-life), Spread.** Each column has a small gray hint line under its label (e.g. *Reversion → "expected hold time"*; *Spread → "buy→sell %"*) so a non-technical user can scan it without reading this README. The full cheat sheet for reading the image is above in *How to read the alert*.

---

## Running the App

**Requirements:** Go 1.25+

**1. Configure** — copy the template and fill in your Discord webhook URL:
```bash
cp .env.example .env
# edit .env → set DISCORD_WEBHOOK_URL to your webhook
```
`.env` is auto-loaded on startup (godotenv) and is git-ignored. `.env.example` is the committed template.

**2. Run the 24/7 service:**
```bash
go run .
```
- Fetches all currencies hourly and stores them in a persistent DuckDB file (`DUCKDB_PATH`, default `./data/prices.duckdb`).
- Every **4 hours** runs the analysis, renders a styled PNG, and posts it to Discord as a file attachment.

**3. Manual commands (flags):**

| Flag | What it does |
|------|--------------|
| `-print` | Print every stored snapshot and exit |
| `-wipe` | Delete the storage file and exit (start over) |
| `-analyze` | Run the analysis once and print the table (no send) |
| `-backtest` | Backtest the buy/sell multipliers on `PriceLogs` and print the best (k, j) per currency, then exit |
| `-send` | Run the real analysis once and post to Discord |
| `-send-test` | Post a sample PNG to verify your webhook works |

**Config (`.env`):**
- `LEAGUE` — league slug (default `runes`)
- `DUCKDB_PATH` — persistent DB file (default `./data/prices.duckdb`)
- `DISCORD_WEBHOOK_URL` — webhook for alerts (required for `-send`/`-send-test`/scheduled alerts). Accepts **comma-separated** URLs to fan the same PNG out to multiple channels (e.g. `https://discord.com/.../A,https://discord.com/.../B`); each is posted independently.

**Cold start:** analysis needs ~48h of accumulated hourly snapshots before it produces real buy/sell targets. Until then `-analyze`/`-send` report `not enough history yet (<48h)`. Use `-send-test` to verify Discord connectivity immediately.

**Tests:**
```bash
go test ./...
```
Covers API parsing (incl. `PriceLogs`), snapshot storage, the analysis math (incl. cold-start guard), the OU half-life filter, the band backtest simulation, and the Discord embed format.

**Data storage:** everything lives in one DuckDB file. `-wipe` (or deleting the file) resets it.
