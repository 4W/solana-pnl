package tests

import (
	"reflect"
	"solana-pnl/internal/pnl"
	"testing"
)

func TestPartitionSpans(t *testing.T) {
	got := pnl.PartitionSpans(10, 10, 4)
	if len(got) != 1 || got[0].Gte != 10 || got[0].Lte != 10 {
		t.Fatalf("single slot: got %+v", got)
	}
	got = pnl.PartitionSpans(0, 100, 3)
	if len(got) != 3 {
		t.Fatalf("len got %d %+v", len(got), got)
	}
	covered := mergeCoverage(got)
	if covered[0] != 0 || covered[1] != 100 {
		t.Fatalf("coverage %v", covered)
	}
}

func mergeCoverage(spans []pnl.SlotSpan) [2]uint64 {
	if len(spans) == 0 {
		return [2]uint64{}
	}
	min := spans[0].Gte
	max := spans[0].Lte
	for _, s := range spans[1:] {
		if s.Gte < min {
			min = s.Gte
		}
		if s.Lte > max {
			max = s.Lte
		}
	}
	return [2]uint64{min, max}
}

func TestPartitionSpansContiguous(t *testing.T) {
	got := pnl.PartitionSpans(0, 99, 5)
	for i := 1; i < len(got); i++ {
		if got[i].Gte != got[i-1].Lte+1 {
			t.Fatalf("gap/overlap at %d: prev %+v curr %+v", i, got[i-1], got[i])
		}
	}
	if got[0].Gte != 0 || got[len(got)-1].Lte != 99 {
		t.Fatalf("bounds: %+v", got)
	}
}

func TestPartitionSpansReflectEqual(t *testing.T) {
	a := pnl.PartitionSpans(100, 200, 8)
	b := pnl.PartitionSpans(100, 200, 8)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("non-deterministic")
	}
}
