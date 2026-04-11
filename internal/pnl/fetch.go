package pnl

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"solana-pnl/internal/helius"
)

const (
	txPageLimit      = 100
	MinSlotSpanSplit = 2
)

var maxTxVer = 0

type SlotSpan struct {
	Gte uint64
	Lte uint64
}

func PartitionCount(concurrency int) int {
	if concurrency < 1 {
		concurrency = 8
	}
	k := concurrency / 2
	return min(max(k, 32), 512)
}

func slotFilters(gte, lte uint64) *helius.GTFAFilters {
	return &helius.GTFAFilters{
		Status:        "any",
		TokenAccounts: "none",
		Slot:          &helius.SlotFilter{Gte: &gte, Lte: &lte},
	}
}

func fullTxOpts(gte, lte uint64, paginationToken string) helius.GetTransactionsForAddressOpts {
	o := helius.GetTransactionsForAddressOpts{
		TransactionDetails:             "full",
		SortOrder:                      "asc",
		Limit:                          txPageLimit,
		Encoding:                       "jsonParsed",
		MaxSupportedTransactionVersion: &maxTxVer,
		Filters:                        slotFilters(gte, lte),
	}
	if paginationToken != "" {
		o.PaginationToken = paginationToken
	}
	return o
}

func DiscoverSlotBounds(ctx context.Context, c *helius.Client, address string) (minSlot, maxSlot uint64, empty bool, err error) {
	base := helius.GetTransactionsForAddressOpts{
		TransactionDetails:             "signatures",
		Limit:                          1,
		MaxSupportedTransactionVersion: &maxTxVer,
		Filters: &helius.GTFAFilters{
			Status:        "any",
			TokenAccounts: "none",
		},
	}

	var ascRes, descRes struct {
		Data []struct {
			Slot uint64 `json:"slot"`
		} `json:"data"`
	}

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
		return 0, 0, false, fmt.Errorf("slot bounds: %w", err)
	}

	if len(ascRes.Data) == 0 {
		return 0, 0, true, nil
	}
	minSlot = ascRes.Data[0].Slot
	if len(descRes.Data) == 0 {
		maxSlot = minSlot
	} else {
		maxSlot = descRes.Data[0].Slot
	}
	if maxSlot < minSlot {
		maxSlot = minSlot
	}
	return minSlot, maxSlot, false, nil
}

func FetchAllTransactions(ctx context.Context, c *helius.Client, address string, minSlot, maxSlot uint64, partitions int) ([]helius.TransactionRow, error) {
	if minSlot > maxSlot {
		return nil, nil
	}
	if partitions < 1 {
		partitions = 16
	}

	spans := PartitionSpans(minSlot, maxSlot, partitions)
	var mu sync.Mutex
	var all []helius.TransactionRow

	g, ctx := errgroup.WithContext(ctx)
	for _, s := range spans {
		s := s
		g.Go(func() error {
			rows, err := fetchRange(ctx, c, address, s.Gte, s.Lte, true)
			if err != nil {
				return err
			}
			mu.Lock()
			all = append(all, rows...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return all, nil
}

func PartitionSpans(minSlot, maxSlot uint64, k int) []SlotSpan {
	if k < 1 {
		k = 1
	}
	width := (maxSlot - minSlot + 1) / uint64(k)
	if width == 0 {
		width = 1
	}
	out := make([]SlotSpan, 0, k)
	cur := minSlot
	for i := 0; i < k && cur <= maxSlot; i++ {
		end := cur + width - 1
		if end > maxSlot {
			end = maxSlot
		}
		if i == k-1 {
			end = maxSlot
		}
		out = append(out, SlotSpan{Gte: cur, Lte: end})
		cur = end + 1
	}
	return out
}

func fetchRange(ctx context.Context, c *helius.Client, address string, gte, lte uint64, adapt bool) ([]helius.TransactionRow, error) {
	res, err := c.GetTransactionsForAddressPage(ctx, address, fullTxOpts(gte, lte, ""))
	if err != nil {
		return nil, err
	}

	data := res.Data
	if len(data) < txPageLimit || res.PaginationToken == nil || *res.PaginationToken == "" {
		return data, nil
	}
	tok := *res.PaginationToken
	mid := gte + (lte-gte)/2
	if !adapt || lte-gte+1 < MinSlotSpanSplit || mid == gte {
		rest, err := fetchSequentialPages(ctx, c, address, gte, lte, tok)
		if err != nil {
			return nil, err
		}
		return append(data, rest...), nil
	}

	g, ctx := errgroup.WithContext(ctx)
	var left, right []helius.TransactionRow
	g.Go(func() error {
		var err error
		left, err = fetchRange(ctx, c, address, gte, mid, adapt)
		return err
	})
	g.Go(func() error {
		if mid+1 > lte {
			return nil
		}
		var err error
		right, err = fetchRange(ctx, c, address, mid+1, lte, adapt)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func fetchSequentialPages(ctx context.Context, c *helius.Client, address string, gte, lte uint64, tok string) ([]helius.TransactionRow, error) {
	if tok == "" {
		return nil, nil
	}
	var out []helius.TransactionRow
	for tok != "" {
		res, err := c.GetTransactionsForAddressPage(ctx, address, fullTxOpts(gte, lte, tok))
		if err != nil {
			return nil, err
		}
		out = append(out, res.Data...)
		if res.PaginationToken == nil || *res.PaginationToken == "" {
			break
		}
		tok = *res.PaginationToken
	}
	return out, nil
}
