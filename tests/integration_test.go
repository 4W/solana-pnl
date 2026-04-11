package tests

import (
	"context"
	"os"
	"testing"

	"solana-pnl/internal/helius"
	"solana-pnl/internal/pnl"
)

const testWallet = "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1"

func TestIntegrationOnePageBalance(t *testing.T) {
	if os.Getenv("RPC_URL") == "" {
		t.Skip("set RPC_URL")
	}
	ctx := context.Background()
	c, _, err := helius.NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	v := 0
	res, err := c.GetTransactionsForAddressPage(ctx, testWallet, helius.GetTransactionsForAddressOpts{
		TransactionDetails:             "full",
		SortOrder:                      "asc",
		Limit:                          5,
		Encoding:                       "jsonParsed",
		MaxSupportedTransactionVersion: &v,
		Filters: &helius.GTFAFilters{
			Status:        "any",
			TokenAccounts: "none",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) == 0 {
		t.Fatal("no data")
	}
	series, err := pnl.BuildBalanceSeries(testWallet, res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != len(res.Data) {
		t.Fatalf("points %d vs rows %d", len(series), len(res.Data))
	}
	for _, p := range series {
		if p.Signature == "" {
			t.Fatal("empty signature")
		}
	}
}
