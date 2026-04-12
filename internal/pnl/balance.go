package pnl

import (
	"encoding/json"
	"fmt"

	"github.com/bytedance/sonic"

	"solana-pnl/internal/helius"
)

func FullAccountKeys(txJSON []byte, meta *helius.TxMeta) ([]string, error) {
	static, err := staticAccountKeysFromTransaction(txJSON)
	if err != nil {
		return nil, err
	}
	if meta == nil || meta.LoadedAddresses == nil {
		return static, nil
	}
	n := len(static) + len(meta.LoadedAddresses.Writable) + len(meta.LoadedAddresses.Readonly)
	out := make([]string, 0, n)
	out = append(out, static...)
	out = append(out, meta.LoadedAddresses.Writable...)
	out = append(out, meta.LoadedAddresses.Readonly...)
	return out, nil
}

func staticAccountKeysFromTransaction(txJSON []byte) ([]string, error) {
	var top map[string]json.RawMessage
	if err := sonic.Unmarshal(txJSON, &top); err != nil {
		return nil, err
	}
	rawMsg, ok := top["message"]
	if !ok {
		return nil, fmt.Errorf("missing message")
	}

	var msgObj map[string]json.RawMessage
	if err := sonic.Unmarshal(rawMsg, &msgObj); err == nil && msgObj != nil {
		if keys, ok := parseAccountKeyArray(msgObj["accountKeys"]); ok {
			return keys, nil
		}
		if keys, ok := parseAccountKeyArray(msgObj["staticAccountKeys"]); ok {
			return keys, nil
		}
	}

	var arr []json.RawMessage
	if err := sonic.Unmarshal(rawMsg, &arr); err == nil && len(arr) >= 2 {
		var inner map[string]json.RawMessage
		if err := sonic.Unmarshal(arr[1], &inner); err == nil {
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
	if err := sonic.Unmarshal(raw, &asStrings); err == nil {
		return asStrings, true
	}
	var mixed []json.RawMessage
	if err := sonic.Unmarshal(raw, &mixed); err != nil {
		return nil, false
	}
	out := make([]string, 0, len(mixed))
	for _, el := range mixed {
		var s string
		if err := sonic.Unmarshal(el, &s); err == nil {
			out = append(out, s)
			continue
		}
		var o struct {
			Pubkey string `json:"pubkey"`
		}
		if err := sonic.Unmarshal(el, &o); err == nil && o.Pubkey != "" {
			out = append(out, o.Pubkey)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
