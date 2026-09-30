package reflow

import (
	"reflect"
	"testing"
)

// 同分场景：展示金额相同时，依次由总历时、交易笔数、交易 ID 序列决定顺序，
// 且结果不受装载顺序影响。
func TestTieBreakers(t *testing.T) {
	txs := []Transaction{
		// 链 A：历时 1000，4 笔，ID 以 a 开头。
		tx("a1", "S", "A", 10000, t0),
		tx("a2", "A", "B", 10000, t0+100),
		tx("a3", "B", "C", 10000, t0+200),
		tx("a4", "C", "S", 10000, t0+1000),
		// 链 B：历时 2000，4 笔。
		tx("b1", "S", "D", 10000, t0),
		tx("b2", "D", "E", 10000, t0+100),
		tx("b3", "E", "F", 10000, t0+200),
		tx("b4", "F", "S", 10000, t0+2000),
		// 链 C：历时 1000，5 笔。
		tx("c1", "S", "G", 10000, t0),
		tx("c2", "G", "H", 10000, t0+100),
		tx("c3", "H", "I", 10000, t0+200),
		tx("c4", "I", "J", 10000, t0+300),
		tx("c5", "J", "S", 10000, t0+1000),
		// 链 D1/D2：历时 1000，4 笔，仅末笔 ID 不同。
		tx("d1", "S", "K", 10000, t0),
		tx("d2", "K", "L", 10000, t0+100),
		tx("d3", "L", "M", 10000, t0+200),
		tx("d4a", "M", "S", 10000, t0+1000),
		tx("d4b", "M", "S", 10000, t0+1000),
	}
	want := [][]string{
		{"a1", "a2", "a3", "a4"},       // 历时 1000、4 笔、ID 最小
		{"d1", "d2", "d3", "d4a"},      // 历时 1000、4 笔、ID 次之
		{"d1", "d2", "d3", "d4b"},      // 历时 1000、4 笔、ID 再次之
		{"c1", "c2", "c3", "c4", "c5"}, // 历时 1000、5 笔
		{"b1", "b2", "b3", "b4"},       // 历时 2000
	}

	check := func(name string, chains []Chain) {
		t.Helper()
		if len(chains) != len(want) {
			t.Fatalf("%s: 候选数量 = %d, 期望 %d; 实际: %v", name, len(chains), len(want), keysOf(chains))
		}
		for i, c := range chains {
			if !reflect.DeepEqual(idsOf(c), want[i]) {
				t.Fatalf("%s: 第 %d 条 = %v, 期望 %v", name, i, idsOf(c), want[i])
			}
		}
	}

	check("正序装载", NewGraph(txs).FindChains("S"))

	// 反序装载结果必须一致。
	reversed := make([]Transaction, len(txs))
	for i, t := range txs {
		reversed[len(txs)-1-i] = t
	}
	check("反序装载", NewGraph(reversed).FindChains("S"))
}

// 展示金额不同则优先按展示金额降序，其余维度不参与。
func TestDisplayAmountDominates(t *testing.T) {
	txs := []Transaction{
		// 链小：展示金额 9000，历时短、笔数少。
		tx("s1", "S", "A", 10000, t0),
		tx("s2", "A", "B", 9000, t0+100),
		tx("s3", "B", "C", 9000, t0+200),
		tx("s4", "C", "S", 9000, t0+300),
		// 链大：展示金额 9500，历时长、笔数多。
		tx("l1", "S", "D", 10000, t0),
		tx("l2", "D", "E", 9500, t0+100),
		tx("l3", "E", "F", 9500, t0+200),
		tx("l4", "F", "G", 9500, t0+300),
		tx("l5", "G", "S", 9500, t0+3000),
	}
	chains := NewGraph(txs).FindChains("S")
	if len(chains) != 2 {
		t.Fatalf("候选数量 = %d, 期望 2; 实际: %v", len(chains), keysOf(chains))
	}
	if got := idsOf(chains[0]); !reflect.DeepEqual(got, []string{"l1", "l2", "l3", "l4", "l5"}) {
		t.Fatalf("展示金额大者应排前, 实际: %v", got)
	}
	if chains[0].DisplayAmount != 9500 || chains[1].DisplayAmount != 9000 {
		t.Fatalf("展示金额 = %d, %d; 期望 9500, 9000", chains[0].DisplayAmount, chains[1].DisplayAmount)
	}
}

// 空图、未知账户与无环账户均返回空结果。
func TestEmptyResults(t *testing.T) {
	var nilGraph *Graph
	if chains := nilGraph.FindChains("S"); len(chains) != 0 {
		t.Fatalf("空图应无候选")
	}
	g := NewGraph([]Transaction{
		tx("x1", "A", "B", 1000, t0),
		tx("x2", "B", "C", 1000, t0+1),
	})
	if chains := g.FindChains("S"); len(chains) != 0 {
		t.Fatalf("未知账户应无候选")
	}
	if chains := g.FindChains("C"); len(chains) != 0 {
		t.Fatalf("无环账户应无候选")
	}
}
