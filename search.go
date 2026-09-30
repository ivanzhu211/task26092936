package reflow

import "sort"

// FindChains 从指定起始账户出发查询回流链，返回全局前 20 条候选。
// 候选按展示金额降序、总历时升序、交易笔数升序、交易 ID 序列字典序升序排列。
func (g *Graph) FindChains(start string) []Chain {
	if g == nil {
		return nil
	}
	visited := map[string]bool{start: true}
	path := make([]Transaction, 0, maxChainLen)
	var chains []Chain

	var dfs func(account string)
	dfs = func(account string) {
		if len(path) == maxChainLen {
			return
		}
		for _, edge := range g.byFrom[account] {
			if len(path) > 0 {
				prev := path[len(path)-1]
				// 出边按时间戳升序，超过 24 小时间隔后即可停止。
				if edge.Timestamp > prev.Timestamp+maxGapMs {
					break
				}
				// 后一笔必须严格晚于前一笔。
				if edge.Timestamp <= prev.Timestamp {
					continue
				}
				// 金额：达到前一笔的 90%（含）且不超过前一笔（含），按整数美分精确比较。
				if edge.Amount > prev.Amount || edge.Amount*10 < prev.Amount*9 {
					continue
				}
			}
			if edge.To == start {
				// 回到起点即完成一条链，链长须为 4 至 6 笔。
				if len(path)+1 >= minChainLen {
					chains = append(chains, buildChain(path, edge))
				}
				continue
			}
			if visited[edge.To] {
				continue
			}
			visited[edge.To] = true
			path = append(path, edge)
			dfs(edge.To)
			path = path[:len(path)-1]
			visited[edge.To] = false
		}
	}
	dfs(start)

	return rank(chains)
}

// buildChain 由当前路径与回到起点的末笔交易构造候选链。
func buildChain(path []Transaction, last Transaction) Chain {
	txs := make([]Transaction, 0, len(path)+1)
	txs = append(txs, path...)
	txs = append(txs, last)
	minAmount := txs[0].Amount
	for _, tx := range txs[1:] {
		if tx.Amount < minAmount {
			minAmount = tx.Amount
		}
	}
	return Chain{
		Transactions:  txs,
		DisplayAmount: minAmount,
		Duration:      txs[len(txs)-1].Timestamp - txs[0].Timestamp,
	}
}

// chainIDs 返回链的交易 ID 序列，用于排序与去重。
func chainIDs(c Chain) []string {
	ids := make([]string, len(c.Transactions))
	for i, tx := range c.Transactions {
		ids[i] = tx.ID
	}
	return ids
}

// compareIDs 按 ASCII 字节顺序比较两个 ID 序列。
func compareIDs(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// lessChain 定义候选排序：展示金额降序、总历时升序、交易笔数升序、ID 序列字典序升序。
func lessChain(a, b Chain) bool {
	if a.DisplayAmount != b.DisplayAmount {
		return a.DisplayAmount > b.DisplayAmount
	}
	if a.Duration != b.Duration {
		return a.Duration < b.Duration
	}
	if len(a.Transactions) != len(b.Transactions) {
		return len(a.Transactions) < len(b.Transactions)
	}
	return compareIDs(chainIDs(a), chainIDs(b)) < 0
}

// rank 去重、排序并截取全局前 20 条候选。
func rank(chains []Chain) []Chain {
	sort.Slice(chains, func(i, j int) bool { return lessChain(chains[i], chains[j]) })
	// 交易 ID 序列相同才算同一候选，去重保留首个。
	seen := make(map[string]bool, len(chains))
	out := make([]Chain, 0, maxResults)
	for _, c := range chains {
		key := ""
		for _, tx := range c.Transactions {
			key += tx.ID + "\x00"
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
		if len(out) == maxResults {
			break
		}
	}
	return out
}
