# PoE2 Stat-Arb & Volatility Trading
*Statistical Arbitrage and Volatility Trading in Path of Exile 2*

**UPDATE:** Decided to use the [poe2scout API](https://poe2scout.com/api/swagger#/{Realm}/Leagues/{LeagueName}/Currencies/Pairs/{CurrencyOneItemId}/{CurrencyTwoItemId}/History) to trade on pairs based on the 3 primary bases: Chaos, Exalted, and Divine.

---

## The Pipeline

> **Pull API Data** (every 60 mins) ➔ **Run Math™** (analyze promising currencies) ➔ **Discord Alerts** (send top 5-10 pairs every few hours) ➔ **Manual Execution** (automation is illegal, don't get banned)

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

### 2. Order Book Imbalance (OBI)
If we just blindly follow the bands, we're naive (volatility isn't the only variable in a market, dumbass). We also need to look at the ratio of **buyers** vs **sellers**—the Order Book Imbalance.

* If the market is heavily weighted with **buyers** (closer to 1), we shift our entire volatility bracket upward based on the ratio. *(We keep our buy relatively low to avoid paying the hype premium).*
* If the market is heavily weighted with **sellers** (closer to -1), we shift our entire volatility bracket downward to scoop up that cheap currency.

**Order Book Imbalance (OBI):**

$$
OBI = \frac{V_b - V_s}{V_b + V_s}
$$

*(Where `V_b` is the total volume of active buy orders and `V_s` is the total volume of active sell orders.)*

**The Skew Adjustment:**

$$
Skew = OBI \times 1.5
$$

*(Where 1.5 is a tuning constant.)*

> ***"But, like, what if something just keeps spiraling into oblivion?"***
> 
> We just keep a max position size? Duh?

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

## The Final Formula

Bringing it all together, our dynamic target limits look like this:

**Target Buy:**

$$
Buy_{final} = \mu_{vw} - 2.5\sigma_{vw} + (OBI \times 1.5\sigma_{vw})
$$

**Target Sell:**

$$
Sell_{final} = \mu_{vw} + 1.5\sigma_{vw} + (OBI \times 1.5\sigma_{vw}) + Premium_{bulk}
$$

---

### Disclaimer
*This model does not yet incorporate in-game Gold fees (YET).*
