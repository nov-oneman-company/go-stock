package agent

// four_table_adapter.go — 「四表工作流」候选接入 go-stock 推荐契约的适配层。
//
// 背景：go-stock 的 AI 选股契约是固定 JSON 数组
// [{code,name,rating,reason}]（见 prompt_backtest_engine.go），由外部 agent
// 产出后交 auto_recommend_saver 入库。四表工作流（量化 ∩ 连板 ∩ 催化，
// 见 a-share-agent-workflows/fusion-daily.md）产出的是带「共振层」与
// 「信号共振度」的候选，字段名不同。
//
// 本文件提供纯函数适配，把四表候选翻译成 go-stock 契约，口径与 Python 侧
// four-table-bridge 严格一致，可单测：
//   - rating 由「共振层 + 置信度 + 是否可成交」确定性推导，同一输入必得同一输出；
//   - 置信度 = 信号共振度，不是收益概率，写进 reason 以免误读。

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ParseFourTableCandidates 解析四表候选 JSON 数组
// （bridge 输出 out/candidates.json 的格式，字段名与 FourTableCandidate 对齐）。
func ParseFourTableCandidates(data []byte) ([]FourTableCandidate, error) {
	var items []FourTableCandidate
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FourTableCandidate 是一条四表候选。
type FourTableCandidate struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Tier       string   `json:"tier"`       // T1..T5 共振层
	Confidence float64  `json:"confidence"` // 信号共振度
	InQuant    bool     `json:"in_quant"`   // 是否命中量化腿
	Board      string   `json:"board"`      // 可成交 / 买不进 / 空
	Catalyst   string   `json:"catalyst"`   // hard / policy / gap
	Gaps       []string `json:"gaps"`       // 缺口标注，原样透出
}

const (
	// 置信度不足以支撑对应评级时的降级门槛（与 Python 侧一致）。
	fourTableConfStrong = 0.55
	fourTableConfOK     = 0.35

	ratingStrong  = "强烈看好"
	ratingGood    = "看好"
	ratingNeutral = "中性"

	boardUntradable = "买不进"
)

// fourTableTierRating 共振层 -> 基础评级。
var fourTableTierRating = map[string]string{
	"T1": ratingStrong,
	"T2": ratingStrong,
	"T3": ratingGood,
	"T4": ratingGood,
	"T5": ratingNeutral,
}

// FourTableRatingOf 由共振层 + 置信度 + 可成交推导 go-stock rating。
func FourTableRatingOf(c FourTableCandidate) string {
	rating := fourTableTierRating[c.Tier]
	if rating == "" {
		rating = ratingNeutral
	}
	if rating == ratingStrong && c.Confidence < fourTableConfStrong {
		rating = ratingGood
	}
	if rating == ratingGood && c.Confidence < fourTableConfOK {
		rating = ratingNeutral
	}
	// 连板买不进：不因连板腿把评级拔到最高档。
	if c.Board == boardUntradable && rating == ratingStrong {
		rating = ratingGood
	}
	return rating
}

// FourTableReasonOf 组装一句话理由，保留共振层 / 各腿状态 / 缺口。
func FourTableReasonOf(c FourTableCandidate) string {
	quant := "量化✗"
	if c.InQuant {
		quant = "量化✓"
	}
	board := c.Board
	if board == "" {
		board = "—"
	}
	catalyst := c.Catalyst
	if catalyst == "" {
		catalyst = "gap"
	}
	var b strings.Builder
	b.WriteString(c.Tier)
	b.WriteString("·共振度")
	b.WriteString(formatFloat(c.Confidence))
	b.WriteString("｜")
	b.WriteString(quant)
	b.WriteString(" 连板")
	b.WriteString(board)
	b.WriteString(" 催化")
	b.WriteString(catalyst)
	b.WriteString("｜置信度=信号共振度≠收益概率")
	if len(c.Gaps) > 0 {
		b.WriteString("；缺口:")
		b.WriteString(strings.Join(c.Gaps, ","))
	}
	return b.String()
}

// FourTableToPicks 把四表候选批量翻译成 go-stock 推荐契约，按
// rating 档位降序、置信度降序排序（便于人读与回测）。
func FourTableToPicks(items []FourTableCandidate) []promptBacktestPickJSON {
	out := make([]promptBacktestPickJSON, 0, len(items))
	for _, c := range items {
		out = append(out, promptBacktestPickJSON{
			Code:   normalizePickCode(c.Code),
			Name:   strings.TrimSpace(c.Name),
			Rating: FourTableRatingOf(c),
			Reason: FourTableReasonOf(c),
		})
	}
	// 稳定排序：rating 档位 -> 置信度
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if fourTablePickLess(out[j], out[i], items) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// fourTableRatingRank rating 档位权重。
func fourTableRatingRank(rating string) int {
	switch rating {
	case ratingStrong:
		return 3
	case ratingGood:
		return 2
	default:
		return 1
	}
}

func fourTablePickLess(a, b promptBacktestPickJSON, items []FourTableCandidate) bool {
	ra, rb := fourTableRatingRank(a.Rating), fourTableRatingRank(b.Rating)
	if ra != rb {
		return ra > rb
	}
	return fourTableConfOf(a.Code, items) > fourTableConfOf(b.Code, items)
}

func fourTableConfOf(code string, items []FourTableCandidate) float64 {
	for _, c := range items {
		if c.Code == code {
			return c.Confidence
		}
	}
	return 0
}

// formatFloat 保留 4 位小数，去掉多余的尾随 0（0.4625 -> "0.4625"，0.63 -> "0.63"）。
func formatFloat(v float64) string {
	s := strings.TrimRight(strings.TrimRight(
		strconv.FormatFloat(v, 'f', 4, 64), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}
