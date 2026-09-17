package acp

// recommend 节点（对齐 acp-kernel recommend.ts）：把相邻可压缩范围批量合并到
// minCompressRangeChars 之上——推荐范围永不低于 apply 侧的最小阈值。
//
// #309 修复语义（v0.0.75）：批量按真实字符量（chars）计数（与 apply 侧
// minCompressRange 门同一记账单位——token×4 的估算在 CJK 感知分词下偏差 4 倍，
// 曾导致 nudge 推荐的范围被 apply 侧原子拒绝）；每个返回的批各自越过阈值；
// 低于阈值的尾部并入前一批（允许过冲）；若没有任何批越过阈值则什么都不出——
// 整段都在门下，推荐它只会产生必然被拒的调用（billion-context #847）。

// CompressibleRange 增补字段见 nudge.go；此处为纯函数集。

// rangeChars 范围的真实字符量（无 chars 字段时回退 tokens×4 历史估算）。
func rangeChars(r CompressibleRange) int {
	if r.Chars > 0 {
		return r.Chars
	}
	return r.Tokens * 4
}

// mergeBatch 合并一批范围为单一起止范围。
func mergeBatch(batch []CompressibleRange) CompressibleRange {
	merged := CompressibleRange{
		StartRef: batch[0].StartRef,
		EndRef:   batch[len(batch)-1].EndRef,
	}
	for _, r := range batch {
		merged.Count += r.Count
		merged.Tokens += r.Tokens
		merged.Chars += r.Chars
	}
	return merged
}

// MergeRangesToThreshold 把相邻范围合并为各自越过 minChars 的批（对齐 #309）。
// minChars<=0 或范围为空时原样返回。
func MergeRangesToThreshold(ranges []CompressibleRange, minChars int) []CompressibleRange {
	if minChars <= 0 || len(ranges) == 0 {
		return ranges
	}
	var result []CompressibleRange
	var batch []CompressibleRange
	batchChars := 0
	for _, r := range ranges {
		batch = append(batch, r)
		batchChars += rangeChars(r)
		if batchChars >= minChars {
			result = append(result, mergeBatch(batch))
			batch = nil
			batchChars = 0
		}
	}
	// 尾部：并入前一批（过冲允许）；无前批则整段低于门——什么都不出
	if len(batch) > 0 && len(result) > 0 {
		prev := result[len(result)-1]
		result[len(result)-1] = mergeBatch(append([]CompressibleRange{prev}, batch...))
	}
	return result
}
