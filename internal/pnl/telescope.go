package pnl

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"

	"solana-pnl/internal/helius"
)

var maxTxVer = 0

func TotalLamportsPnL(ctx context.Context, c *helius.Client, address string) (pnlLamports int64, firstSlot, lastSlot uint64, empty bool, err error) {
	base := helius.GetTransactionsForAddressOpts{
		TransactionDetails:             "full",
		Limit:                          1,
		Encoding:                       "jsonParsed",
		MaxSupportedTransactionVersion: &maxTxVer,
		Filters: &helius.GTFAFilters{
			Status:        "any",
			TokenAccounts: "none",
		},
	}

	var ascRes, descRes helius.GetTransactionsForAddressResult

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		o := base
		o.SortOrder = "asc"
		return c.Call(ctx, "getTransactionsForAddress", []any{address, o.ToMap()}, &ascRes)
	})
	g.Go(func() error {
		o := base
		o.SortOrder = "desc"
		return c.Call(ctx, "getTransactionsForAddress", []any{address, o.ToMap()}, &descRes)
	})
	if err := g.Wait(); err != nil {
		return 0, 0, 0, false, err
	}

	if len(ascRes.Data) == 0 {
		return 0, 0, 0, true, nil
	}
	if len(descRes.Data) == 0 {
		return 0, 0, 0, false, fmt.Errorf("descending page empty but ascending had data")
	}

	first := ascRes.Data[0]
	last := descRes.Data[0]

	i0, err := accountIndex(&first, address)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("first tx: %w", err)
	}
	iN, err := accountIndex(&last, address)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("last tx: %w", err)
	}

	preFirst := int64(first.Meta.PreBalances[i0])
	postLast := int64(last.Meta.PostBalances[iN])
	return postLast - preFirst, first.Slot, last.Slot, false, nil
}

func accountIndex(row *helius.TransactionRow, addr string) (int, error) {
	if row.Meta == nil {
		return 0, fmt.Errorf("missing meta")
	}
	keys, err := FullAccountKeys(row.Transaction, row.Meta)
	if err != nil {
		return 0, err
	}
	i := slices.Index(keys, addr)
	if i < 0 {
		return 0, fmt.Errorf("wallet not in account list")
	}
	if i >= len(row.Meta.PreBalances) || i >= len(row.Meta.PostBalances) {
		return 0, fmt.Errorf("balance index out of range")
	}
	return i, nil
}
