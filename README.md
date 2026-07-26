# PoE2 Stat-Arb & Volatility Trading
*Statistical Arbitrage and Volatility Trading in Path of Exile 2*

**UPDATE:** Decided to use the [poe2scout API](https://api.poe2scout.com/swagger/index.html)

Using currency by category to page over currency prices for the past month now.

There is also only per-day data so we cant make per-hour trades like I hoped. It will have to send out after each day completes for the most up-to-date data.
 
Only have daily volume endpoints, BUT the "current price" and "current volume" fields get updated every hour, so I will have to shift from a stateless design to a stateful implementation saving api calls every hour.

`OBI Discontinued`

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

### 2. Volume-Weighted Momentum Skew (VMS)
Since poe2scout doesn't expose a live order book, we cannot look at pending buyer/seller intent. Instead, we look at actual historical aggression—whether short-term price action is ripping or dipping relative to the long-term trend, weighted by volume.

We compare a short-term volume-weighted average ($\mu_{short}$, e.g., last 3 hours) against our baseline long-term average ($\mu_{vw}$, e.g., last 24 hours). To keep our skew bounded between $-1$ and $1$ (just like an OBI model), we run the deviation through a hyperbolic tangent ($\tanh$) function.

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

---

### Disclaimer
*This model does not yet incorporate in-game Gold fees (YET).*

Chaos Orb: 160 Gold (Item ID: chaos)

Exalted Orb: 120 Gold (Item ID: exalted)

Divine Orb: 800 (Item ID: divine)
