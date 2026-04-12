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

## The core idea (the part that actually matters)

There's a clean telescoping identity: net change in lamports across **all** txs for an address is just **`post(last) − pre(first)`** on that address's balance slots in meta , so you only need the **first** and **last** transaction in time, not the whole chain.

That maps to **two** `getTransactionsForAddress` calls (asc `limit: 1` + desc `limit: 1`, full tx + `jsonParsed`), in parallel. Same rough latency whether someone has 200 txs or 200k , you're not paginating history for the number itself.

That's the algorithmic win. Everything else is "make those two round trips not hurt more than they have to."

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
