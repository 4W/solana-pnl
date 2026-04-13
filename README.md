# solana-pnl , Helius mini dev weekend challenge


results from my test home on the main run (cmd/pnl/main.go):
```
C:\Users\syrof\OneDrive\Desktop\solana-pnl>go run cmd/pnl/main.go
DdbBbLpXvLJuyN2d1qnkA5DufojkUxGsdVQmjuZaXknv | PnL: +0.040017121 SOL | slots: 308060352 … 401140586 | 126 ms
CyaE1VxvBrahnPWkqm5VsdCvyS2QmNht2UFrKJHga54o | PnL: +811.044330083 SOL | slots: 317524721 … 412685384 | 87 ms
AuPp4YTMTyqxYXQnHc5KUc6pUuCSsHQpBJhgnD45yqrf | PnL: +219.978264405 SOL | slots: 321788731 … 412627847 | 79 ms
9yYya3F5EJoLnBNKW6z4bZvyQytMXzDcpU5D6yYr4jqL | PnL: +65.711961535 SOL | slots: 265894120 … 412722279 | 81 ms
Bi4rd5FH5bYEN8scZ7wevxNZyNmKHdaBcvewdPFxYdLt | PnL: +221.837605598 SOL | slots: 331094960 … 412730857 | 80 ms
---
wallets: 5 ok | avg lookup: 90 ms | wall: 455 ms
```


Hey, this is my take on the **lowest-latency SOL PnL** challenge: compute PnL from **plain RPC** (no indexer), using **`getTransactionsForAddress`** so you're not stuck with the old one-way `getSignaturesForAddress` crawl.

## What the challenge is asking for

You need **SOL PnL** at runtime with **only RPC**. The tricky part isn't really "math on the balances", it's **how little work you can do on the wire** when you don't know if a wallet is sparse or absolutely slammed with txs.

## Algorithm

**What we compute.** "SOL PnL" here is **net lamport change over the wallet’s full on-chain history**: balance after the latest touch minus balance before the earliest touch. That is a single scalar, not a time series.

**Why you don’t need every transaction.** For one account, the deltas across txs telescope: the sum of all balance changes equals **`postBalances[i]` on the last tx** minus **`preBalances[i]` on the first tx**, where `i` is that account’s index in each transaction’s meta. So total PnL depends only on **boundary** rows, not on how dense or sparse the middle is.

**RPC shape.** With `getTransactionsForAddress` you can ask for exactly those boundaries:

- `sortOrder: "asc"`, `limit: 1` → earliest matching tx for the address  
- `sortOrder: "desc"`, `limit: 1` → latest matching tx for the address  

Run those **in parallel**. Use `transactionDetails: "full"` (or enough to get `meta`), `encoding: "jsonParsed"`, and filters consistent with ""SOL-only" intent (e.g. `tokenAccounts: "none"`). Resolve the wallet's index in the full account key list (including **loaded addresses** on versioned txs), then read `preBalances` / `postBalances` at that index.

**Cost vs history size.** Work on the wire is **two** GTFA calls regardless of whether the wallet has hundreds or hundreds of thousands of transactions—no pagination for the number itself. The rest of the codebase is mostly about making those two round trips fast.

## Where we actually spent effort: the network stack

Honestly, once the telescoping path was in place, **profiling said the CPU was barely awake** , almost all the time was **waiting on I/O** (TLS, HTTP, remote side). So squeezing JSON parsers wouldn't have moved the needle much for *median* latency.

We went pretty hard on **client-side network optimization** instead:

- **`net/http`** with a cloned default transport so **HTTP/2** can actually happen when the endpoint supports it , good fit for **two parallel RPCs** on one connection.
- **Warmup** (`getHealth`) before the timed work so you're not measuring **cold TLS / pool** every time if you care about steady-state.
- **TCP_NODELAY**, tuned **idle connection pool**, bigger **read/write buffers**, **no gzip** on JSON bodies, **TLS session cache**, sensible **ALPN** (`h2` first), faster ECDHE curves where it helps.
- Optional **`http://`** to a **trusted local relay** if you put something in front of the real HTTPS endpoint , zero TLS on the app hop when that's safe.

Multi-wallet mode reuses **one client** so connections stay hot across lookups.

So: **algorithm = tiny number of RPCs; "speed work" = mostly making those RPCs cheap on the network path.**

## Run it

- Copy `.env.example` → `.env`, set **`RPC_URL`** (Helius or whatever speaks the same JSON-RPC).
- `go run ./cmd/pnl` , default is a small list of wallets; pass addresses as args to override.
- Output is **PnL + slot range + ms per wallet** and a short footer with averages.

## Tests / bench

- Integration tests hit the real RPC if `RPC_URL` is set.
- There's a **benchmark** in `tests/` for end-to-end `ns/op` , which i was using heavily to optimize latency. im sure helius's dev team can optimize this further to essentially cut the network latency further since i was running this on my home pc.


---

That's the gist.
