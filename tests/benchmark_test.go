package tests

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"solana-pnl/internal/helius"
	"solana-pnl/internal/pnl"
)

func init() {
	_ = godotenv.Load("../.env")
}

func BenchmarkTotalLamportsPnL(b *testing.B) {
	if os.Getenv("RPC_URL") == "" {
		b.Skip("set RPC_URL")
	}
	ctx := context.Background()
	c, _, err := helius.NewFromEnv()
	if err != nil {
		b.Fatal(err)
	}
	if err := c.Warmup(ctx); err != nil {
		b.Fatal(err)
	}
	wallet := os.Getenv("BENCH_WALLET")
	if wallet == "" {
		wallet = testWallet
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _, err := pnl.TotalLamportsPnL(ctx, c, wallet)
		if err != nil {
			b.Fatal(err)
		}
	}
}
