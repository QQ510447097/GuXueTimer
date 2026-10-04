package timer

import (
	"fmt"
	"time"

	"goxuetimer/internal/store"
)

// Recover 在应用启动时把持久化的计时状态接回现实。
//
// 规则（gap = 现在 - 最后一次心跳时刻）：
//
//	gap ≤ 15 分钟   ：认为只是崩溃/重启，整段计入，继续计时
//	15 分钟 < gap ≤ 8 小时：只补入 15 分钟，其余不计入，继续计时
//	gap > 8 小时    ：认为会话早已结束，按最后一次心跳结算并落库
//
// 因为每次心跳都会把锚点落库，accumulated 本来就是"截至最后一次心跳"的准确值，
// 所以上面补的都是"应用没在跑的那段"要不要认账的问题。
func (s *Service) Recover() (RecoveryOutcome, error) {
	s.mu.Lock()
	out, notice, err := s.recoverLocked()
	snap := s.snap
	stateCB, noticeCB := s.onStateChange, s.onNotice
	s.mu.Unlock()

	if err != nil {
		return out, err
	}
	if notice != "" && noticeCB != nil {
		noticeCB(NoticeWarning, notice)
	}
	if stateCB != nil {
		stateCB(snap)
	}
	return out, nil
}

func (s *Service) recoverLocked() (RecoveryOutcome, string, error) {
	// 载入时发现脏数据：清成 idle 并告知。
	if s.sanitized {
		prev, prevCarry := s.snap, s.carryNano
		s.snap = Idle()
		s.carryNano = 0
		if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
			s.snap, s.carryNano = prev, prevCarry
			return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
		}
		s.sanitized = false
		return RecoveryOutcome{Action: RecoveryReset, Snapshot: s.snap},
			"本地计时状态已损坏，已重置为未计时", nil
	}

	switch s.snap.State {
	case StateIdle:
		return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", nil
	case StatePaused:
		// 暂停态没有"离线时长"的概念，原样恢复即可。
		return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", nil
	}

	now := s.clock()
	gap := now.Sub(s.snap.RunningSince)

	switch {
	case gap < 0:
		// 系统时间被往前调了：不猜，把锚点拉回现在。
		prev := s.snap
		s.snap.RunningSince = now
		if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
			s.snap = prev
			return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
		}
		return RecoveryOutcome{Action: RecoveryContinued, Snapshot: s.snap},
			"检测到系统时间回拨，离线时长未计入", nil

	case gap <= ResumeGraceGap:
		prev, prevCarry := s.snap, s.carryNano
		s.snap.RunningSince = now
		s.creditNanos(gap)
		if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
			s.snap, s.carryNano = prev, prevCarry
			return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
		}
		return RecoveryOutcome{Action: RecoveryContinued, Snapshot: s.snap}, "", nil

	case gap <= ResumeAbandonGap:
		prev, prevCarry := s.snap, s.carryNano
		s.snap.RunningSince = now
		s.creditNanos(ResumeGraceGap)
		if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
			s.snap, s.carryNano = prev, prevCarry
			return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
		}
		skipped := int64((gap - ResumeGraceGap) / time.Second)
		return RecoveryOutcome{
				Action:     RecoveryContinuedClamped,
				SkippedSec: skipped,
				Snapshot:   s.snap,
			},
			fmt.Sprintf("上次计时中断了 %s，只补入 %s，其余未计入",
				humanDuration(gap), humanDuration(ResumeGraceGap)), nil

	default:
		// 中断太久：会话视为在最后一次心跳时已结束。
		startedAt := s.snap.StartedAt
		endedAt := s.snap.RunningSince
		dur := s.snap.AccumulatedSec

		if endedAt.Before(startedAt) {
			endedAt = startedAt.Add(time.Duration(dur) * time.Second)
		}

		if dur < MinSessionSec {
			prev, prevCarry := s.snap, s.carryNano
			s.snap = Idle()
			s.carryNano = 0
			if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
				s.snap, s.carryNano = prev, prevCarry
				return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
			}
			return RecoveryOutcome{
					Action:      RecoveryAutoStopped,
					Saved:       false,
					DurationSec: dur,
					Snapshot:    s.snap,
				},
				"上次计时中断过久且时长过短，该会话已放弃", nil
		}

		rec := store.SessionRow{
			StartedAt:   startedAt.Format(recordLayout),
			EndedAt:     endedAt.Format(recordLayout),
			DurationSec: dur,
			Source:      "recovered",
			CreatedAt:   now.Format(recordLayout),
		}
		// FinishSession 是单事务的：要么记录与状态一起落库，要么都不动。
		id, err := s.st.FinishSession(rec, store.TimerStateRow{State: string(StateIdle)})
		if err != nil {
			return RecoveryOutcome{Action: RecoveryNone, Snapshot: s.snap}, "", err
		}
		s.snap = Idle()
		s.carryNano = 0
		return RecoveryOutcome{
				Action:      RecoveryAutoStopped,
				Saved:       true,
				DurationSec: dur,
				SessionID:   id,
				Snapshot:    s.snap,
			},
			fmt.Sprintf("上次计时中断了 %s，已按中断时刻结算为 %s",
				humanDuration(gap), humanDuration(time.Duration(dur)*time.Second)), nil
	}
}
