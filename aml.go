// Package amlchain provides in-memory temporal cycle (回流链) search
// over a fixed set of transactions for AML investigation tools.
package amlchain

import "sort"

// maxResults is the global cap on returned candidates.
const maxResults = 20

// minLen and maxLen bound the number of transactions per chain.
const minLen, maxLen = 4, 6

// maxGapMs is the maximum allowed gap between adjacent transactions (24h).
const maxGapMs = 24 * 60 * 60 * 1000

// Transaction is a single directed money transfer.
type Transaction struct {
	ID        string // unique ASCII transaction ID
	From      string // payer account
	To        string // payee account
	Amount    int64  // USD amount in integer cents
	Timestamp int64  // UTC milliseconds
}

// Chain is one detected cycle: an ordered list of transaction IDs
// starting and ending at the queried account.
type Chain struct {
	TxIDs      []string
	MinAmount  int64 // smallest single amount in the chain (cents)
	DurationMs int64 // last.Timestamp - first.Timestamp
}

// Store holds an immutable, loaded transaction set.
type Store struct {
	txs []Transaction
	out map[string][]int // account -> indices of outgoing transactions
}

// Load builds a Store from the given transactions.
func Load(txs []Transaction) *Store {
	s := &Store{
		txs: make([]Transaction, len(txs)),
		out: make(map[string][]int),
	}
	copy(s.txs, txs)
	for i := range s.txs {
		s.out[s.txs[i].From] = append(s.out[s.txs[i].From], i)
	}
	return s
}

// FindCycles returns up to 20 best cycles of 4-6 transactions that
// start and end at account, ordered by the candidate ranking rules.
func (s *Store) FindCycles(account string) []Chain {
	var chains []Chain
	path := make([]int, 0, maxLen)
	visited := map[string]bool{account: true}

	var dfs func(account string)
	dfs = func(cur string) {
		for _, idx := range s.out[cur] {
			tx := s.txs[idx]
			if len(path) > 0 {
				prev := s.txs[path[len(path)-1]]
				if tx.Timestamp <= prev.Timestamp || tx.Timestamp-prev.Timestamp > maxGapMs {
					continue
				}
				if tx.Amount > prev.Amount || tx.Amount*10 < prev.Amount*9 {
					continue
				}
			}
			if tx.To == account {
				// Closing edge: only valid once the chain has minLen txs.
				if len(path)+1 >= minLen {
					chains = append(chains, s.buildChain(path, idx))
				}
				continue
			}
			if visited[tx.To] || len(path)+1 >= maxLen {
				continue
			}
			visited[tx.To] = true
			path = append(path, idx)
			dfs(tx.To)
			path = path[:len(path)-1]
			delete(visited, tx.To)
		}
	}
	dfs(account)

	sort.Slice(chains, func(i, j int) bool { return less(chains[i], chains[j]) })
	if len(chains) > maxResults {
		chains = chains[:maxResults]
	}
	return chains
}

// buildChain materializes a Chain from the current path plus a closing edge.
func (s *Store) buildChain(path []int, last int) Chain {
	idxs := make([]int, 0, len(path)+1)
	idxs = append(idxs, path...)
	idxs = append(idxs, last)

	c := Chain{TxIDs: make([]string, len(idxs))}
	c.MinAmount = s.txs[idxs[0]].Amount
	for k, idx := range idxs {
		tx := s.txs[idx]
		c.TxIDs[k] = tx.ID
		if tx.Amount < c.MinAmount {
			c.MinAmount = tx.Amount
		}
	}
	c.DurationMs = s.txs[idxs[len(idxs)-1]].Timestamp - s.txs[idxs[0]].Timestamp
	return c
}

// less orders candidates: MinAmount desc, DurationMs asc, tx count asc,
// then transaction ID sequence in ASCII lexicographic order.
func less(a, b Chain) bool {
	if a.MinAmount != b.MinAmount {
		return a.MinAmount > b.MinAmount
	}
	if a.DurationMs != b.DurationMs {
		return a.DurationMs < b.DurationMs
	}
	if len(a.TxIDs) != len(b.TxIDs) {
		return len(a.TxIDs) < len(b.TxIDs)
	}
	for k := range a.TxIDs {
		if a.TxIDs[k] != b.TxIDs[k] {
			return a.TxIDs[k] < b.TxIDs[k]
		}
	}
	return false
}
