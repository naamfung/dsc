package session

import "fmt"

// 第 7 步「fork」：从稳定前缀创建子会话（对齐 DSH SessionStore.fork）。
// 子会话深拷贝事件日志到 boundary（含）并重建 surface；前缀不得结束于
// 开放轮次/步骤内（拒绝静默截断）。

// Fork 复制事件日志到 boundary（含）创建新会话。
// 返回的会话与原会话共享事件对象（事件不可变：append-only，无人改写），
// 但持有独立的 events 切片与 surface 投影，后续追加互不影响。
// boundary 超出范围或前缀结束于开放轮次/步骤内时返回错误。
func (s *Session) Fork(boundary int) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if boundary < 0 || boundary >= len(s.events) {
		return nil, fmt.Errorf("session: fork boundary %d out of range [0, %d)", boundary, len(s.events))
	}
	prefix := s.events[:boundary+1]
	var turns, turnEnds, steps, stepEnds int
	for _, ev := range prefix {
		switch ev.Type {
		case TurnStart:
			turns++
		case TurnEnd:
			turnEnds++
		case StepStart:
			steps++
		case StepEnd:
			stepEnds++
		}
	}
	if turns != turnEnds || steps != stepEnds {
		return nil, fmt.Errorf("session: fork boundary %d ends inside an open turn/step", boundary)
	}
	child := &Session{}
	child.events = append([]*Event(nil), prefix...)
	for _, ev := range prefix {
		if ev.Surface != nil {
			child.applySurfaceLocked(ev)
		}
	}
	return child, nil
}

// LastTurn 返回日志中出现的最大轮次号（无则 0），供恢复后继续编号。
func (s *Session) LastTurn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	max := 0
	for _, ev := range s.events {
		if ev.Type == TurnStart {
			if d, ok := ev.Data.(*TurnData); ok && d.Turn > max {
				max = d.Turn
			}
		}
	}
	return max
}
