"""Monte Carlo check of Apex Forge v0.3.0 verdict rules (learner.compare + outcome.evaluate).

Two IDENTICAL setups, 17 corners, n push passes per corner per setup, Gaussian corner-time
noise with sd = 28 ms (median push-pass scatter on Jason's real FA26 COTA log).
Per corner: median difference vs noise = max(15 ms, 2 * SE) with SE from 1.4826 * MAD,
exactly as learner.compare does. Reports how often "slower in 2+ corners and more slower than
faster" fires, which outcome.evaluate turns into a Regression when the target didn't improve.

Run: python tools/verdict_noise_sim.py   (prints 39%, 33%, 26% for n = 3, 5, 8)
"""

import math
import random
import statistics as st


def mad(x):
    m = st.median(x)
    return st.median([abs(v - m) for v in x])


def run(n=3, corners=17, sd=0.028, eff=0.0, trials=4000, seed=1):
    rnd = random.Random(seed)
    hits = 0
    for _ in range(trials):
        slower = faster = 0
        for _ in range(corners):
            a = [rnd.gauss(0, sd) for _ in range(n)]
            b = [rnd.gauss(eff, sd) for _ in range(n)]
            sa, sb = 1.4826 * mad(a), 1.4826 * mad(b)
            noise = max(0.015, 2 * math.sqrt(sa * sa / n + sb * sb / n))
            d = st.median(b) - st.median(a)
            if d > noise:
                slower += 1
            elif d < -noise:
                faster += 1
        if slower >= 2 and slower > faster:
            hits += 1
    return hits / trials


if __name__ == "__main__":
    for n in (3, 5, 8):
        print(f"n={n}: identical setups flagged slowerWins {run(n=n):.0%}")
