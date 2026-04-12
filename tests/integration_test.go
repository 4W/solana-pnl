package tests

import (
	"context"
	"os"
	"slices"
	"testing"

	"solana-pnl/internal/helius"
	"solana-pnl/internal/pnl"
)

const testWallet = "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1"

func TestIntegrationTelescopePnL(t *testing.T) {
	if os.Getenv("RPC_URL") == "" {
		t.Skip("set RPC_URL")
	}
	ctx := context.Background()
	c, _, err := helius.NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	lamports, firstSlot, lastSlot, empty, err := pnl.TotalLamportsPnL(ctx, c, testWallet)
	if err != nil {
		t.Fatal(err)
	}
	if empty {
		t.Fatal("expected non-empty wallet")
	}
	if firstSlot == 0 || lastSlot == 0 || firstSlot > lastSlot {
		t.Fatalf("unexpected slots: %d … %d", firstSlot, lastSlot)
	}
	_ = lamports
}

func TestIntegrationAccountKeysFromPage(t *testing.T) {
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
	for _, row := range res.Data {
		if row.Meta == nil {
			t.Fatal("nil meta")
		}
		keys, err := pnl.FullAccountKeys(row.Transaction, row.Meta)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(keys, testWallet) {
			t.Fatal("wallet not in keys")
		}
	}
}
