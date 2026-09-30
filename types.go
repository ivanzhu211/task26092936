// Package reflow 提供反洗钱时序回流链检索能力。
// 交易数据装载后保存在进程内存中，不再变化。
package reflow

import "sort"

// Transaction 表示一笔交易。
type Transaction struct {
	ID        string // 唯一 ASCII 交易 ID
	From      string // 付款账户
	To        string // 收款账户
	Amount    int64  // 金额，整数美分
	Timestamp int64  // UTC 毫秒时间戳
}

// Chain 表示一条回流链候选。
type Chain struct {
	Transactions  []Transaction // 按链顺序排列的交易
	DisplayAmount int64         // 展示金额：链中最小单笔金额
	Duration      int64         // 总历时：末笔与首笔的时间差（毫秒）
}

// Graph 是装载后的交易集合，支持按起始账户查询回流链。
type Graph struct {
	byFrom map[string][]Transaction // 按付款账户索引的出边
}

// NewGraph 装载交易集合。一次装载后数据不再变化。
func NewGraph(txs []Transaction) *Graph {
	g := &Graph{byFrom: make(map[string][]Transaction, len(txs))}
	for _, tx := range txs {
		g.byFrom[tx.From] = append(g.byFrom[tx.From], tx)
	}
	// 按时间戳、交易 ID 排序，保证遍历顺序确定且便于按时间剪枝。
	for from := range g.byFrom {
		edges := g.byFrom[from]
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].Timestamp != edges[j].Timestamp {
				return edges[i].Timestamp < edges[j].Timestamp
			}
			return edges[i].ID < edges[j].ID
		})
	}
	return g
}

const (
	minChainLen = 4                   // 链最少交易笔数
	maxChainLen = 6                   // 链最多交易笔数
	maxGapMs    = 24 * 60 * 60 * 1000 // 相邻交易最大间隔：24 小时（含边界）
	maxResults  = 20                  // 返回候选上限
)
