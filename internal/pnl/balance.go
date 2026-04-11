package pnl

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"solana-pnl/internal/helius"
)

type BalancePoint struct {
	Slot             uint64
	TransactionIndex int
	BlockTime        *int64
	Signature        string
	LamportsDelta    int64
	LamportsAfter    uint64
}

func BuildBalanceSeries(wallet string, rows []helius.TransactionRow) ([]BalancePoint, error) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Slot != b.Slot {
			return a.Slot < b.Slot
		}
		return a.TransactionIndex < b.TransactionIndex
	})

	out := make([]BalancePoint, 0, len(rows))
	for _, row := range rows {
		if row.Meta == nil {
			continue
		}
		keys, err := FullAccountKeys(row.Transaction, row.Meta)
		if err != nil {
			return nil, fmt.Errorf("slot %d txIdx %d: %w", row.Slot, row.TransactionIndex, err)
		}
		idx := slices.Index(keys, wallet)
		if idx < 0 {
			return nil, fmt.Errorf("wallet %s not in account list (slot %d idx %d)", wallet, row.Slot, row.TransactionIndex)
		}
		if idx >= len(row.Meta.PreBalances) || idx >= len(row.Meta.PostBalances) {
			return nil, fmt.Errorf("balance index out of range: %d", idx)
		}
		delta := int64(row.Meta.PostBalances[idx]) - int64(row.Meta.PreBalances[idx])
		sig, _ := signatureFromTx(row.Transaction)
		after := row.Meta.PostBalances[idx]

		out = append(out, BalancePoint{
			Slot:             row.Slot,
			TransactionIndex: row.TransactionIndex,
			BlockTime:        row.BlockTime,
			Signature:        sig,
			LamportsDelta:    delta,
			LamportsAfter:    after,
		})
	}
	return out, nil
}

func FullAccountKeys(txJSON []byte, meta *helius.TxMeta) ([]string, error) {
	static, err := staticAccountKeysFromTransaction(txJSON)
	if err != nil {
		return nil, err
	}
	if meta == nil || meta.LoadedAddresses == nil {
		return static, nil
	}
	out := make([]string, 0, len(static)+len(meta.LoadedAddresses.Writable)+len(meta.LoadedAddresses.Readonly))
	out = append(out, static...)
	out = append(out, meta.LoadedAddresses.Writable...)
	out = append(out, meta.LoadedAddresses.Readonly...)
	return out, nil
}

func staticAccountKeysFromTransaction(txJSON []byte) ([]string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(txJSON, &top); err != nil {
		return nil, err
	}
	rawMsg, ok := top["message"]
	if !ok {
		return nil, fmt.Errorf("missing message")
	}

	var msgObj map[string]json.RawMessage
	if err := json.Unmarshal(rawMsg, &msgObj); err == nil && msgObj != nil {
		if keys, ok := parseAccountKeyArray(msgObj["accountKeys"]); ok {
			return keys, nil
		}
		if keys, ok := parseAccountKeyArray(msgObj["staticAccountKeys"]); ok {
			return keys, nil
		}
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(rawMsg, &arr); err == nil && len(arr) >= 2 {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(arr[1], &inner); err == nil {
			if keys, ok := parseAccountKeyArray(inner["accountKeys"]); ok {
				return keys, nil
			}
			if keys, ok := parseAccountKeyArray(inner["staticAccountKeys"]); ok {
				return keys, nil
			}
		}
	}

	return nil, fmt.Errorf("could not parse account keys from message")
}

func parseAccountKeyArray(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		return asStrings, true
	}
	var mixed []json.RawMessage
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return nil, false
	}
	out := make([]string, 0, len(mixed))
	for _, el := range mixed {
		var s string
		if err := json.Unmarshal(el, &s); err == nil {
			out = append(out, s)
			continue
		}
		var o struct {
			Pubkey string `json:"pubkey"`
		}
		if err := json.Unmarshal(el, &o); err == nil && o.Pubkey != "" {
			out = append(out, o.Pubkey)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func signatureFromTx(txJSON []byte) (string, error) {
	var top struct {
		Signatures []string `json:"signatures"`
	}
	if err := json.Unmarshal(txJSON, &top); err != nil {
		return "", err
	}
	if len(top.Signatures) == 0 {
		return "", fmt.Errorf("no signatures")
	}
	return top.Signatures[0], nil
}
