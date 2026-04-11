package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"

	"solana-pnl/internal/helius"
	"solana-pnl/internal/pnl"
)

const defaultAddress = "BqjxAhvmj1aK6GrcrqfHMaASKPxFL9eSne1HXEfxpRde"

func main() {
	_ = godotenv.Load()

	addr := flag.String("address", defaultAddress, "wallet pubkey (base58)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	c, conc, err := helius.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		os.Exit(1)
	}
	k := pnl.PartitionCount(conc)

	minSlot, maxSlot, empty, err := pnl.DiscoverSlotBounds(ctx, c, *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bounds: %v\n", err)
		os.Exit(1)
	}
	if empty {
		fmt.Println("no transactions for address")
		return
	}

	span := maxSlot - minSlot + 1
	fmt.Fprintf(os.Stderr, "[pnl] slot range [%d, %d] (%d slots); starting fetch (K=%d partitions, adapt=true)\n",
		minSlot, maxSlot, span, k)
	if span > 10_000_000 {
		fmt.Fprintf(os.Stderr, "[pnl] warning: very large activity window — full history can take hours or days.\n")
	}
	fmt.Fprintf(os.Stderr, "[pnl] progress: stderr heartbeat every 15s while fetching (10m deadline)…\n")

	t0 := time.Now()
	stopBeat := make(chan struct{})
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stopBeat:
				return
			case <-tick.C:
				fmt.Fprintf(os.Stderr, "[pnl] still fetching… %s elapsed\n", time.Since(t0).Round(time.Second))
			}
		}
	}()

	rows, err := pnl.FetchAllTransactions(ctx, c, *addr, minSlot, maxSlot, k)
	close(stopBeat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
		os.Exit(1)
	}
	fetchDur := time.Since(t0)

	t1 := time.Now()
	series, err := pnl.BuildBalanceSeries(*addr, rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "balance: %v\n", err)
		os.Exit(1)
	}
	buildDur := time.Since(t1)

	meta := map[string]any{
		"address":          *addr,
		"minSlot":          minSlot,
		"maxSlot":          maxSlot,
		"transactionCount": len(rows),
		"points":           len(series),
		"fetchMs":          fetchDur.Milliseconds(),
		"buildMs":          buildDur.Milliseconds(),
		"totalMs":          time.Since(t0).Milliseconds(),
	}

	fmt.Fprintf(os.Stderr, "# %+v\n", meta)
	fmt.Println("slot,tx_index,block_time_unix,signature,lamports_delta,lamports_after")
	for _, p := range series {
		bt := ""
		if p.BlockTime != nil {
			bt = fmt.Sprintf("%d", *p.BlockTime)
		}
		fmt.Printf("%d,%d,%s,%s,%d,%d\n", p.Slot, p.TransactionIndex, bt, p.Signature, p.LamportsDelta, p.LamportsAfter)
	}
}
