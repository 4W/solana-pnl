package helius

import "encoding/json"

type GetTransactionsForAddressOpts struct {
	TransactionDetails             string
	SortOrder                      string
	Limit                          int
	PaginationToken                string
	Encoding                       string
	MaxSupportedTransactionVersion *int
	Filters                        *GTFAFilters
}

type GTFAFilters struct {
	Slot          *SlotFilter `json:"slot,omitempty"`
	Status        string      `json:"status,omitempty"`
	TokenAccounts string      `json:"tokenAccounts,omitempty"`
}

type SlotFilter struct {
	Gte *uint64 `json:"gte,omitempty"`
	Gt  *uint64 `json:"gt,omitempty"`
	Lte *uint64 `json:"lte,omitempty"`
	Lt  *uint64 `json:"lt,omitempty"`
}

func (o GetTransactionsForAddressOpts) ToMap() map[string]any {
	m := map[string]any{}
	if o.TransactionDetails != "" {
		m["transactionDetails"] = o.TransactionDetails
	}
	if o.SortOrder != "" {
		m["sortOrder"] = o.SortOrder
	}
	if o.Limit > 0 {
		m["limit"] = o.Limit
	}
	if o.PaginationToken != "" {
		m["paginationToken"] = o.PaginationToken
	}
	if o.Encoding != "" {
		m["encoding"] = o.Encoding
	}
	if o.MaxSupportedTransactionVersion != nil {
		m["maxSupportedTransactionVersion"] = *o.MaxSupportedTransactionVersion
	}
	if o.Filters != nil {
		f := map[string]any{}
		if o.Filters.Slot != nil {
			sf := map[string]any{}
			if o.Filters.Slot.Gte != nil {
				sf["gte"] = *o.Filters.Slot.Gte
			}
			if o.Filters.Slot.Gt != nil {
				sf["gt"] = *o.Filters.Slot.Gt
			}
			if o.Filters.Slot.Lte != nil {
				sf["lte"] = *o.Filters.Slot.Lte
			}
			if o.Filters.Slot.Lt != nil {
				sf["lt"] = *o.Filters.Slot.Lt
			}
			f["slot"] = sf
		}
		if o.Filters.Status != "" {
			f["status"] = o.Filters.Status
		}
		if o.Filters.TokenAccounts != "" {
			f["tokenAccounts"] = o.Filters.TokenAccounts
		}
		m["filters"] = f
	}
	return m
}

type GetTransactionsForAddressResult struct {
	Data            []TransactionRow `json:"data"`
	PaginationToken *string          `json:"paginationToken"`
}

type TransactionRow struct {
	Slot             uint64          `json:"slot"`
	TransactionIndex int             `json:"transactionIndex"`
	BlockTime        *int64          `json:"blockTime"`
	Transaction      json.RawMessage `json:"transaction"`
	Meta             *TxMeta         `json:"meta"`
}

type TxMeta struct {
	Err             json.RawMessage  `json:"err"`
	PreBalances     []uint64         `json:"preBalances"`
	PostBalances    []uint64         `json:"postBalances"`
	LoadedAddresses *LoadedAddresses `json:"loadedAddresses"`
}

type LoadedAddresses struct {
	Writable []string `json:"writable"`
	Readonly []string `json:"readonly"`
}
