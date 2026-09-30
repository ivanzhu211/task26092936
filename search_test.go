package reflow

import (
	"fmt"
	"reflect"
	"testing"
)

const t0 = 1700000000000 // 基准时间戳（毫秒）

func tx(id, from, to string, amount, ts int64) Transaction {
	return Transaction{ID: id, From: from, To: to, Amount: amount, Timestamp: ts}
}

// idsOf 返回链的交易 ID 序列。
func idsOf(c Chain) []string {
	ids := make([]string, len(c.Transactions))
	for i, t := range c.Transactions {
		ids[i] = t.ID
	}
	return ids
}

// keysOf 返回所有候选的 ID 序列连接串，用于集合比较与重复检测。
func keysOf(chains []Chain) []string {
	keys := make([]string, len(chains))
	for i, c := range chains {
		k := ""
		for _, t := range c.Transactions {
			k += t.ID + "|"
		}
		keys[i] = k
	}
	return keys
}

// expectChainSet 核对候选集合与期望完全一致且无重复。
func expectChainSet(t *testing.T, chains []Chain, want [][]string) {
	t.Helper()
	if len(chains) != len(want) {
		t.Fatalf("候选数量 = %d, 期望 %d; 实际: %v", len(chains), len(want), keysOf(chains))
	}
	seen := map[string]bool{}
	got := map[string]bool{}
	for i, c := range chains {
		k := keysOf(chains)[i]
		if seen[k] {
			t.Fatalf("候选重复: %v", idsOf(c))
		}
		seen[k] = true
		got[k] = true
	}
	for _, w := range want {
		k := ""
		for _, id := range w {
			k += id + "|"
		}
		if !got[k] {
			t.Fatalf("缺少期望候选 %v; 实际: %v", w, keysOf(chains))
		}
	}
}

// 结构成环但时序倒置，不应返回任何候选。
func TestTimeInvertedCycle(t *testing.T) {
	g := NewGraph([]Transaction{
		tx("t1", "S", "A", 1000, t0+3000),
		tx("t2", "A", "B", 1000, t0+2000),
		tx("t3", "B", "C", 1000, t0+1000),
		tx("t4", "C", "S", 1000, t0),
	})
	if chains := g.FindChains("S"); len(chains) != 0 {
		t.Fatalf("时序倒置应无候选, 实际: %v", keysOf(chains))
	}
}

// 相邻间隔恰好 24 小时合法，超过 1 毫秒即非法。
func TestGapBoundary24h(t *testing.T) {
	const day = 24 * 60 * 60 * 1000
	g := NewGraph([]Transaction{
		// 合法链：S->A 与 A->B 间隔恰好 24h。
		tx("g1", "S", "A", 1000, t0),
		tx("g2", "A", "B", 1000, t0+day),
		tx("g3", "B", "C", 1000, t0+day+1),
		tx("g4", "C", "S", 1000, t0+day+2),
		// 非法链：S->X 与 X->Y 间隔 24h+1ms。
		tx("h1", "S", "X", 1000, t0),
		tx("h2", "X", "Y", 1000, t0+day+1),
		tx("h3", "Y", "Z", 1000, t0+day+2),
		tx("h4", "Z", "S", 1000, t0+day+3),
	})
	expectChainSet(t, g.FindChains("S"), [][]string{{"g1", "g2", "g3", "g4"}})
}

// 后一笔金额恰好为前一笔 90% 合法，少 1 美分即非法。
func TestAmountBoundary90(t *testing.T) {
	g := NewGraph([]Transaction{
		// 合法链：1000 -> 900 恰好 90%。
		tx("a1", "S", "A", 1000, t0),
		tx("a2", "A", "B", 900, t0+1),
		tx("a3", "B", "C", 900, t0+2),
		tx("a4", "C", "S", 900, t0+3),
		// 非法链：1000 -> 899 低于 90%。
		tx("b1", "S", "P", 1000, t0),
		tx("b2", "P", "Q", 899, t0+1),
		tx("b3", "Q", "R", 899, t0+2),
		tx("b4", "R", "S", 899, t0+3),
	})
	expectChainSet(t, g.FindChains("S"), [][]string{{"a1", "a2", "a3", "a4"}})
}

// 后一笔金额等于前一笔（100%）合法，超过 1 美分即非法。
func TestAmountBoundary100(t *testing.T) {
	g := NewGraph([]Transaction{
		// 合法链：金额保持不变。
		tx("e1", "S", "A", 1000, t0),
		tx("e2", "A", "B", 1000, t0+1),
		tx("e3", "B", "C", 1000, t0+2),
		tx("e4", "C", "S", 1000, t0+3),
		// 非法链：后一笔金额超过前一笔。
		tx("f1", "S", "P", 1000, t0),
		tx("f2", "P", "Q", 1001, t0+1),
		tx("f3", "Q", "R", 1001, t0+2),
		tx("f4", "R", "S", 1001, t0+3),
	})
	expectChainSet(t, g.FindChains("S"), [][]string{{"e1", "e2", "e3", "e4"}})
}

// 平行交易分别参与搜索；不同候选允许共享交易。
func TestParallelAndSharedTransactions(t *testing.T) {
	g := NewGraph([]Transaction{
		tx("s1", "S", "A", 5000, t0),
		tx("p1", "A", "B", 5000, t0+1), // 平行交易 1
		tx("p2", "A", "B", 5000, t0+2), // 平行交易 2
		tx("s2", "B", "C", 5000, t0+3),
		tx("s3", "C", "S", 5000, t0+4),
	})
	chains := g.FindChains("S")
	expectChainSet(t, chains, [][]string{
		{"s1", "p1", "s2", "s3"},
		{"s1", "p2", "s2", "s3"},
	})
	// 两条候选共享 s1/s2/s3，不得因共享被排除。
	for _, c := range chains {
		if idsOf(c)[0] != "s1" || idsOf(c)[3] != "s3" {
			t.Fatalf("候选未共享首尾交易: %v", idsOf(c))
		}
	}
}

// 链长边界：3 笔与 7 笔不成候选，4/5/6 笔合法；中间账户不得重复。
func TestChainLengthAndDistinctAccounts(t *testing.T) {
	g := NewGraph([]Transaction{
		tx("e1", "S", "A", 10000, t0),
		tx("e2", "A", "B", 10000, t0+1),
		tx("e3", "B", "S", 10000, t0+2), // 3 笔环，非法
		tx("e4", "B", "C", 10000, t0+2),
		tx("e5", "C", "S", 10000, t0+3), // 4 笔，合法
		tx("e6", "C", "D", 10000, t0+3),
		tx("e7", "D", "S", 10000, t0+4), // 5 笔，合法
		tx("e8", "D", "E", 10000, t0+4),
		tx("e9", "E", "S", 10000, t0+5), // 6 笔，合法
		tx("e10", "E", "F", 10000, t0+5),
		tx("e11", "F", "S", 10000, t0+6), // 7 笔，非法
		// 中间账户重复（P 出现两次），非法。
		tx("r1", "S", "P", 10000, t0+100),
		tx("r2", "P", "Q", 10000, t0+101),
		tx("r3", "Q", "P", 10000, t0+102),
		tx("r4", "P", "R", 10000, t0+103),
		tx("r5", "R", "S", 10000, t0+104),
	})
	expectChainSet(t, g.FindChains("S"), [][]string{
		{"e1", "e2", "e4", "e5"},
		{"e1", "e2", "e4", "e6", "e7"},
		{"e1", "e2", "e4", "e6", "e8", "e9"},
	})
}

// 超过 20 条候选时，按展示金额降序精确返回前 20 条。
func TestTop20Ordering(t *testing.T) {
	txs := []Transaction{
		tx("E0", "S", "A", 100000, t0),
		tx("E1", "A", "B", 99000, t0+1000),
		tx("E2", "B", "C", 98000, t0+2000),
	}
	// 25 条平行末笔，展示金额 97000 递减至 94600。
	want := [][]string{}
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("L%02d", i)
		txs = append(txs, tx(id, "C", "S", 97000-int64(i)*100, t0+3000))
		if i < 20 {
			want = append(want, []string{"E0", "E1", "E2", id})
		}
	}
	chains := NewGraph(txs).FindChains("S")
	if len(chains) != 20 {
		t.Fatalf("候选数量 = %d, 期望 20", len(chains))
	}
	for i, c := range chains {
		if !reflect.DeepEqual(idsOf(c), want[i]) {
			t.Fatalf("第 %d 条 = %v, 期望 %v", i, idsOf(c), want[i])
		}
		if c.DisplayAmount != 97000-int64(i)*100 {
			t.Fatalf("第 %d 条展示金额 = %d, 期望 %d", i, c.DisplayAmount, 97000-int64(i)*100)
		}
		if c.Duration != 3000 {
			t.Fatalf("第 %d 条总历时 = %d, 期望 3000", i, c.Duration)
		}
	}
}
