package agent

import "testing"

func TestFourTableRatingOf(t *testing.T) {
	cases := []struct {
		name string
		in   FourTableCandidate
		want string
	}{
		{"T1高置信度->强烈看好", FourTableCandidate{Tier: "T1", Confidence: 0.9}, ratingStrong},
		{"T1低置信度降级->看好", FourTableCandidate{Tier: "T1", Confidence: 0.5}, ratingGood},
		{"T2低置信度降级->看好", FourTableCandidate{Tier: "T2", Confidence: 0.4}, ratingGood},
		{"T3置信度不足->中性", FourTableCandidate{Tier: "T3", Confidence: 0.3}, ratingNeutral},
		{"T4足够->看好", FourTableCandidate{Tier: "T4", Confidence: 0.63}, ratingGood},
		{"T5->中性", FourTableCandidate{Tier: "T5", Confidence: 0.9}, ratingNeutral},
		{"未知层->中性", FourTableCandidate{Tier: "T9", Confidence: 0.9}, ratingNeutral},
		{"买不进不拔高", FourTableCandidate{Tier: "T1", Confidence: 0.9, Board: boardUntradable}, ratingGood},
	}
	for _, c := range cases {
		if got := FourTableRatingOf(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestFourTableReasonCarriesDiscipline(t *testing.T) {
	c := FourTableCandidate{
		Code: "603928", Name: "兴业股份", Tier: "T4", Confidence: 0.63,
		InQuant: true, Board: "可成交", Catalyst: "gap", Gaps: []string{"催化腿缺口"},
	}
	reason := FourTableReasonOf(c)
	for _, want := range []string{"T4", "共振度", "0.63", "量化✓", "置信度=信号共振度≠收益概率", "催化腿缺口"} {
		if !contains(reason, want) {
			t.Errorf("reason 缺 %q：%s", want, reason)
		}
	}
}

func TestFourTableToPicksSortsAndNormalizes(t *testing.T) {
	items := []FourTableCandidate{
		{Code: "600000", Name: "浦发银行", Tier: "T5", Confidence: 0.9},
		{Code: "sh603928", Name: "兴业股份", Tier: "T4", Confidence: 0.63, InQuant: true, Board: "可成交"},
		{Code: "002009", Name: "天奇股份", Tier: "T3", Confidence: 0.7, InQuant: true},
	}
	picks := FourTableToPicks(items)
	if len(picks) != 3 {
		t.Fatalf("len=%d", len(picks))
	}
	// T3/T4 -> 看好 排在 T5 -> 中性 之前
	if picks[0].Rating != ratingGood || picks[2].Rating != ratingNeutral {
		t.Errorf("排序错误: %+v", picks)
	}
	// 代码归一化：sh603928 -> 603928
	found := false
	for _, p := range picks {
		if p.Code == "603928" && p.Name == "兴业股份" {
			found = true
		}
	}
	if !found {
		t.Errorf("未归一化代码: %+v", picks)
	}
}

func TestParseFourTableCandidates(t *testing.T) {
	data := []byte(`[{"code":"603928","name":"兴业股份","tier":"T4","confidence":0.63,"in_quant":true,"board":"可成交","catalyst":"gap","gaps":["催化腿缺口"]}]`)
	items, err := ParseFourTableCandidates(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].Code != "603928" || !items[0].InQuant {
		t.Fatalf("解析错误: %+v", items)
	}
	if got := FourTableRatingOf(items[0]); got != ratingGood {
		t.Errorf("rating=%q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
