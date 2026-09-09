package core

import (
	"testing"
)

func TestGoalRoundDriverActivate(t *testing.T) {
	d := NewGoalRoundDriver(DefaultGoalRoundConfig())
	if d.Active() {
		t.Error("should not be active initially")
	}
	d.Activate()
	if !d.Active() {
		t.Error("should be active after Activate")
	}
	if !d.ShouldContinue() {
		t.Error("should continue after activate")
	}
}

func TestGoalRoundDriverDisarm(t *testing.T) {
	d := NewGoalRoundDriver(DefaultGoalRoundConfig())
	d.Activate()
	d.Disarm()
	if d.ShouldContinue() {
		t.Error("should not continue after disarm")
	}
}

func TestGoalRoundDriverMaxRounds(t *testing.T) {
	d := NewGoalRoundDriver(GoalRoundConfig{
		MaxGoalRounds:   2,
		GoalRoundPrompt: "Round {round}/{max}",
	})
	d.Activate()

	// 第一轮
	if !d.ShouldContinue() {
		t.Error("should continue round 1")
	}
	prompt := d.Advance()
	if prompt != "Round 1/2" {
		t.Errorf("prompt = %q, want 'Round 1/2'", prompt)
	}

	// 第二轮
	if !d.ShouldContinue() {
		t.Error("should continue round 2")
	}
	prompt = d.Advance()
	if prompt != "Round 2/2" {
		t.Errorf("prompt = %q, want 'Round 2/2'", prompt)
	}

	// 第三轮应被阻止
	if d.ShouldContinue() {
		t.Error("should not continue round 3 (max=2)")
	}
}

func TestGoalRoundDriverUnlimitedRounds(t *testing.T) {
	d := NewGoalRoundDriver(GoalRoundConfig{
		MaxGoalRounds:   0, // 无限制
		GoalRoundPrompt: "Round {round}/{max}",
	})
	d.Activate()

	for i := 0; i < 100; i++ {
		if !d.ShouldContinue() {
			t.Fatalf("should continue at round %d (unlimited)", i+1)
		}
		d.Advance()
	}
	if d.Rounds() != 100 {
		t.Errorf("rounds = %d, want 100", d.Rounds())
	}
}

func TestGoalRoundDriverComplete(t *testing.T) {
	d := NewGoalRoundDriver(DefaultGoalRoundConfig())
	d.Activate()
	d.Complete()
	if d.Active() {
		t.Error("should not be active after Complete")
	}
	if d.ShouldContinue() {
		t.Error("should not continue after Complete")
	}
}

func TestGoalRoundDriverInfinitePrompt(t *testing.T) {
	d := NewGoalRoundDriver(GoalRoundConfig{
		MaxGoalRounds:   0,
		GoalRoundPrompt: "round {round} of {max}",
	})
	d.Activate()
	prompt := d.Advance()
	if prompt != "round 1 of ∞" {
		t.Errorf("prompt = %q, want 'round 1 of ∞'", prompt)
	}
}
