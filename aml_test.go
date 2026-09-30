package amlchain

import (
	"reflect"
	"testing"
)

const (
	t0   = int64(1700000000000)
	hour = int64(3600000)
	min  = int64(60000)
	day  = 24 * hour
)

func tx(id, from, to string, amount, ts int64) Transaction {
	return Transaction{ID: id, From: from, To: to, Amount: amount, Timestamp: ts}
}

func ids(chains []Chain) [][]string {
	out := make([][]string, len(chains))
	for i, c := range chains {
		out[i] = c.TxIDs
	}
	return out
}

func TestTemporalOrderRequired(t *testing.T) {
	valid := []Transaction{
		tx("e1", "A", "B", 10000, t0),
		tx("e2", "B", "C", 10000, t0+hour),
		tx("e3", "C", "D", 10000, t0+2*hour),
		tx("e4", "D", "A", 10000, t0+3*hour),
	}
	got := Load(valid).FindCycles("A")
	if len(got) != 1 || !reflect.DeepEqual(got[0].TxIDs, []string{"e1", "e2", "e3", "e4"}) {
		t.Fatalf("valid cycle not found: %+v", got)
	}
	if got[0].MinAmount != 10000 || got[0].DurationMs != 3*hour {
		t.Fatalf("wrong chain metrics: %+v", got[0])
	}

	// Same ring, but e3 predates e2: structurally cyclic, temporally inverted.
	inverted := []Transaction{
		tx("e1", "A", "B", 10000, t0),
		tx("e2", "B", "C", 10000, t0+hour),
		tx("e3", "C", "D", 10000, t0-hour),
		tx("e4", "D", "A", 10000, t0+3*hour),
	}
	if got := Load(inverted).FindCycles("A"); len(got) != 0 {
		t.Fatalf("inverted cycle must be rejected, got %+v", got)
	}
}

func TestGapBoundary(t *testing.T) {
	// Every adjacent gap is exactly 24h: allowed (boundary inclusive).
	exact := []Transaction{
		tx("g1", "A", "B", 10000, t0),
		tx("g2", "B", "C", 10000, t0+day),
		tx("g3", "C", "D", 10000, t0+2*day),
		tx("g4", "D", "A", 10000, t0+3*day),
	}
	got := Load(exact).FindCycles("A")
	if len(got) != 1 || got[0].DurationMs != 3*day {
		t.Fatalf("24h boundary gap must be allowed: %+v", got)
	}

	// One gap is 24h+1ms: rejected.
	over := []Transaction{
		tx("g1", "A", "B", 10000, t0),
		tx("g2", "B", "C", 10000, t0+day),
		tx("g3", "C", "D", 10000, t0+2*day+1),
		tx("g4", "D", "A", 10000, t0+3*day+1),
	}
	if got := Load(over).FindCycles("A"); len(got) != 0 {
		t.Fatalf("gap above 24h must be rejected: %+v", got)
	}
}

func TestAmountRatioBoundary(t *testing.T) {
	// Each hop is exactly 90% of the previous: allowed.
	exact := []Transaction{
		tx("a1", "A", "B", 10000, t0),
		tx("a2", "B", "C", 9000, t0+hour),
		tx("a3", "C", "D", 8100, t0+2*hour),
		tx("a4", "D", "A", 7290, t0+3*hour),
	}
	got := Load(exact).FindCycles("A")
	if len(got) != 1 || got[0].MinAmount != 7290 {
		t.Fatalf("exact 90%% steps must be allowed: %+v", got)
	}

	// Cent-exact ratio: 90% of 10001 is 9000.9, so 9001 passes, 9000 fails.
	pass := []Transaction{
		tx("a1", "A", "B", 10001, t0),
		tx("a2", "B", "C", 9001, t0+hour),
		tx("a3", "C", "D", 9001, t0+2*hour),
		tx("a4", "D", "A", 9001, t0+3*hour),
	}
	if got := Load(pass).FindCycles("A"); len(got) != 1 {
		t.Fatalf("9001 >= 90%% of 10001 must pass: %+v", got)
	}
	fail := []Transaction{
		tx("a1", "A", "B", 10001, t0),
		tx("a2", "B", "C", 9000, t0+hour),
		tx("a3", "C", "D", 9000, t0+2*hour),
		tx("a4", "D", "A", 9000, t0+3*hour),
	}
	if got := Load(fail).FindCycles("A"); len(got) != 0 {
		t.Fatalf("9000 < 90%% of 10001 must fail: %+v", got)
	}

	// 100% boundary: equal amount allowed, any increase rejected.
	equal := []Transaction{
		tx("a1", "A", "B", 10000, t0),
		tx("a2", "B", "C", 10000, t0+hour),
		tx("a3", "C", "D", 10000, t0+2*hour),
		tx("a4", "D", "A", 10000, t0+3*hour),
	}
	if got := Load(equal).FindCycles("A"); len(got) != 1 {
		t.Fatalf("equal amounts (100%%) must be allowed: %+v", got)
	}
	growth := []Transaction{
		tx("a1", "A", "B", 10000, t0),
		tx("a2", "B", "C", 10001, t0+hour),
		tx("a3", "C", "D", 10001, t0+2*hour),
		tx("a4", "D", "A", 10001, t0+3*hour),
	}
	if got := Load(growth).FindCycles("A"); len(got) != 0 {
		t.Fatalf("amount increase must be rejected: %+v", got)
	}
}

func TestParallelAndSharedTransactions(t *testing.T) {
	// Two parallel A->B edges, two parallel paths B->...->D, one shared D->A edge.
	txs := []Transaction{
		tx("t01", "A", "B", 10000, t0),
		tx("t02", "A", "B", 10000, t0),
		tx("t11", "B", "C", 10000, t0+hour),
		tx("t21", "C", "D", 10000, t0+2*hour),
		tx("t12", "B", "E", 10000, t0+hour),
		tx("t22", "E", "D", 10000, t0+2*hour),
		tx("t31", "D", "A", 10000, t0+3*hour),
	}
	got := Load(txs).FindCycles("A")
	want := [][]string{
		{"t01", "t11", "t21", "t31"},
		{"t01", "t12", "t22", "t31"},
		{"t02", "t11", "t21", "t31"},
		{"t02", "t12", "t22", "t31"},
	}
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("parallel/shared candidates wrong:\n got %v\nwant %v", ids(got), want)
	}
	// No duplicate candidates.
	seen := map[string]bool{}
	for _, c := range got {
		key := ""
		for _, id := range c.TxIDs {
			key += id + "|"
		}
		if seen[key] {
			t.Fatalf("duplicate candidate %v", c.TxIDs)
		}
		seen[key] = true
	}
}

func TestTop20Order(t *testing.T) {
	// 3*2*2*2 = 24 valid candidates, all with identical amount, duration
	// and length, so the result must be the first 20 in ID lexicographic order.
	var txs []Transaction
	for _, id := range []string{"a1", "a2", "a3"} {
		txs = append(txs, tx(id, "A", "B", 10000, t0))
	}
	for _, id := range []string{"b1", "b2"} {
		txs = append(txs, tx(id, "B", "C", 10000, t0+hour))
	}
	for _, id := range []string{"c1", "c2"} {
		txs = append(txs, tx(id, "C", "D", 10000, t0+2*hour))
	}
	for _, id := range []string{"d1", "d2"} {
		txs = append(txs, tx(id, "D", "A", 10000, t0+3*hour))
	}

	var want [][]string
	for _, a := range []string{"a1", "a2", "a3"} {
		for _, b := range []string{"b1", "b2"} {
			for _, c := range []string{"c1", "c2"} {
				for _, d := range []string{"d1", "d2"} {
					want = append(want, []string{a, b, c, d})
				}
			}
		}
	}
	want = want[:20]

	got := Load(txs).FindCycles("A")
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("top-20 order wrong:\n got %v\nwant %v", ids(got), want)
	}
}

func TestTieBreaking(t *testing.T) {
	// All chains below share MinAmount 10000 except the last (9000).
	txs := []Transaction{
		// len 4, duration 3h, IDs t01..t04
		tx("t01", "S", "a", 10000, t0),
		tx("t02", "a", "b", 10000, t0+hour),
		tx("t03", "b", "c", 10000, t0+2*hour),
		tx("t04", "c", "S", 10000, t0+3*hour),
		// len 4, duration 3h, IDs t11..t14 (loses to t01.. on IDs)
		tx("t11", "S", "d", 10000, t0),
		tx("t12", "d", "e", 10000, t0+hour),
		tx("t13", "e", "f", 10000, t0+2*hour),
		tx("t14", "f", "S", 10000, t0+3*hour),
		// len 5, duration 3h (loses to len-4 chains on tx count)
		tx("t21", "S", "g", 10000, t0),
		tx("t22", "g", "h", 10000, t0+45*min),
		tx("t23", "h", "i", 10000, t0+90*min),
		tx("t24", "i", "j", 10000, t0+135*min),
		tx("t25", "j", "S", 10000, t0+180*min),
		// len 4, duration 4h (loses on duration)
		tx("t31", "S", "k", 10000, t0),
		tx("t32", "k", "l", 10000, t0+hour),
		tx("t33", "l", "m", 10000, t0+2*hour),
		tx("t34", "m", "S", 10000, t0+4*hour),
		// MinAmount 9000, duration 1h: amount dominates, comes last.
		tx("t41", "S", "n", 10000, t0),
		tx("t42", "n", "o", 9500, t0+15*min),
		tx("t43", "o", "p", 9100, t0+30*min),
		tx("t44", "p", "S", 9000, t0+hour),
	}
	want := [][]string{
		{"t01", "t02", "t03", "t04"},
		{"t11", "t12", "t13", "t14"},
		{"t21", "t22", "t23", "t24", "t25"},
		{"t31", "t32", "t33", "t34"},
		{"t41", "t42", "t43", "t44"},
	}
	got := Load(txs).FindCycles("S")
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("tie-breaking wrong:\n got %v\nwant %v", ids(got), want)
	}

	// Load order must not affect the result.
	rev := make([]Transaction, len(txs))
	for i, x := range txs {
		rev[len(txs)-1-i] = x
	}
	gotRev := Load(rev).FindCycles("S")
	if !reflect.DeepEqual(ids(gotRev), want) {
		t.Fatalf("load order changed result:\n got %v\nwant %v", ids(gotRev), want)
	}
}

func TestChainLengthBounds(t *testing.T) {
	// 6-transaction cycle: allowed.
	six := []Transaction{
		tx("x1", "S", "a", 10000, t0),
		tx("x2", "a", "b", 10000, t0+hour),
		tx("x3", "b", "c", 10000, t0+2*hour),
		tx("x4", "c", "d", 10000, t0+3*hour),
		tx("x5", "d", "e", 10000, t0+4*hour),
		tx("x6", "e", "S", 10000, t0+5*hour),
	}
	got := Load(six).FindCycles("S")
	if len(got) != 1 || len(got[0].TxIDs) != 6 {
		t.Fatalf("6-transaction cycle must be found: %+v", got)
	}

	// 3-transaction cycle: too short.
	three := []Transaction{
		tx("y1", "S", "a", 10000, t0),
		tx("y2", "a", "b", 10000, t0+hour),
		tx("y3", "b", "S", 10000, t0+2*hour),
	}
	if got := Load(three).FindCycles("S"); len(got) != 0 {
		t.Fatalf("3-transaction cycle must be rejected: %+v", got)
	}

	// 7-transaction cycle: too long.
	var seven []Transaction
	nodes := []string{"S", "a", "b", "c", "d", "e", "f"}
	for i := 0; i < 7; i++ {
		seven = append(seven, tx("z"+string(rune('1'+i)), nodes[i], nodes[(i+1)%7], 10000, t0+int64(i)*hour))
	}
	if got := Load(seven).FindCycles("S"); len(got) != 0 {
		t.Fatalf("7-transaction cycle must be rejected: %+v", got)
	}

	// No cycle at all / empty store.
	if got := Load(nil).FindCycles("S"); len(got) != 0 {
		t.Fatalf("empty store must yield no chains: %+v", got)
	}
}
