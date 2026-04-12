package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"solana-pnl/internal/helius"
	"solana-pnl/internal/pnl"
)

var defaultWallets = []string{
	"DdbBbLpXvLJuyN2d1qnkA5DufojkUxGsdVQmjuZaXknv",
	"CyaE1VxvBrahnPWkqm5VsdCvyS2QmNht2UFrKJHga54o",
	"AuPp4YTMTyqxYXQnHc5KUc6pUuCSsHQpBJhgnD45yqrf",
	"9yYya3F5EJoLnBNKW6z4bZvyQytMXzDcpU5D6yYr4jqL",
	"Bi4rd5FH5bYEN8scZ7wevxNZyNmKHdaBcvewdPFxYdLt",
}

func main() {
	_ = godotenv.Load()

	args := os.Args[1:]
	wallets := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a != "" {
			wallets = append(wallets, a)
		}
	}
	if len(wallets) == 0 {
		wallets = defaultWallets
	}

	c, _, err := helius.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Duration(len(wallets)) * 90 * time.Second
	if deadline < 2*time.Minute {
		deadline = 2 * time.Minute
	}
	if deadline > 30*time.Minute {
		deadline = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	if err := c.Warmup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warmup: %v\n", err)
		os.Exit(1)
	}

	runStart := time.Now()
	var sumMs int64
	var okN, failN int

	for _, addr := range wallets {
		t0 := time.Now()
		pnlLamports, firstSlot, lastSlot, empty, err := pnl.TotalLamportsPnL(ctx, c, addr)
		ms := time.Since(t0).Milliseconds()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", addr, err)
			failN++
			continue
		}
		sumMs += ms
		okN++
		if empty {
			fmt.Printf("%s | PnL: 0.000000000 SOL | slots: — … — | %d ms\n", addr, ms)
			continue
		}
		sol := float64(pnlLamports) / 1e9
		sign := ""
		if pnlLamports >= 0 {
			sign = "+"
		}
		fmt.Printf("%s | PnL: %s%.9f SOL | slots: %d … %d | %d ms\n", addr, sign, sol, firstSlot, lastSlot, ms)
	}

	totalMs := time.Since(runStart).Milliseconds()
	fmt.Printf("---\nwallets: %d ok", okN)
	if failN > 0 {
		fmt.Printf(", %d failed", failN)
	}
	if okN > 0 {
		fmt.Printf(" | avg lookup: %d ms", sumMs/int64(okN))
	}
	fmt.Printf(" | wall: %d ms\n", totalMs)

	if failN > 0 {
		os.Exit(1)
	}
}
