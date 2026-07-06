package economy

import (
	"sync"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// fakeClock lets tests control "now" deterministically.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{t: start}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestClaimDailyRefill_TooEarlyDenied(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := NewLedger(NewMemoryStore(), clock.now)

	granted, ok := l.ClaimDailyRefill("p1")
	if !ok {
		t.Fatalf("first claim should succeed")
	}
	if granted != DailyRefill {
		t.Fatalf("first claim grant = %d, want %d", granted, DailyRefill)
	}

	clock.advance(23 * time.Hour)
	if _, ok := l.ClaimDailyRefill("p1"); ok {
		t.Fatalf("claim before 24h elapsed should be denied")
	}
}

func TestClaimDailyRefill_StreakIncrementsOnConsecutiveDays(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := NewLedger(NewMemoryStore(), clock.now)

	if _, ok := l.ClaimDailyRefill("p1"); !ok {
		t.Fatalf("first claim should succeed")
	}

	clock.advance(30 * time.Hour) // within 24-48h -> streak++
	granted, ok := l.ClaimDailyRefill("p1")
	if !ok {
		t.Fatalf("second claim within 24-48h should succeed")
	}
	want := DailyRefill + 1*streakBonusUnit
	if granted != want {
		t.Fatalf("grant = %d, want %d", granted, want)
	}

	pe, _ := l.store.Get("p1")
	if pe.Streak != 1 {
		t.Fatalf("streak = %d, want 1", pe.Streak)
	}
}

func TestClaimDailyRefill_StreakResetsAfterGap(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := NewLedger(NewMemoryStore(), clock.now)

	l.ClaimDailyRefill("p1")
	clock.advance(30 * time.Hour)
	l.ClaimDailyRefill("p1") // streak = 1

	clock.advance(72 * time.Hour) // gap > 48h -> reset
	granted, ok := l.ClaimDailyRefill("p1")
	if !ok {
		t.Fatalf("claim after long gap should still succeed")
	}
	if granted != DailyRefill {
		t.Fatalf("grant after reset = %d, want %d", granted, DailyRefill)
	}
	pe, _ := l.store.Get("p1")
	if pe.Streak != 0 {
		t.Fatalf("streak = %d, want 0 after reset", pe.Streak)
	}
}

func TestClaimDailyRefill_GrantCappedAtTwiceDailyRefill(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := NewLedger(NewMemoryStore(), clock.now)

	l.ClaimDailyRefill("p1") // streak 0 -> grant DailyRefill
	for i := 0; i < 5; i++ {
		clock.advance(30 * time.Hour)
		granted, ok := l.ClaimDailyRefill("p1")
		if !ok {
			t.Fatalf("claim %d should succeed", i)
		}
		if cap := 2 * DailyRefill; granted > cap {
			t.Fatalf("grant %d exceeds cap %d", granted, cap)
		}
	}
	pe, _ := l.store.Get("p1")
	if pe.Balance <= 0 {
		t.Fatalf("balance should be positive, got %d", pe.Balance)
	}
}

func TestBuyIn_InsufficientFunds(t *testing.T) {
	clock := newFakeClock(time.Now())
	l := NewLedger(NewMemoryStore(), clock.now)

	// Balance starts at StartingBalance; try to buy in for more than that.
	err := l.BuyIn("p1", StartingBalance+1)
	if err != ErrInsufficientFunds {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if bal := l.Balance("p1"); bal != StartingBalance {
		t.Fatalf("balance should be unchanged after failed buy-in, got %d", bal)
	}
}

func TestBuyIn_CashOut_RoundTrip(t *testing.T) {
	clock := newFakeClock(time.Now())
	l := NewLedger(NewMemoryStore(), clock.now)

	if err := l.BuyIn("p1", 4_000); err != nil {
		t.Fatalf("BuyIn failed: %v", err)
	}
	if bal := l.Balance("p1"); bal != StartingBalance-4_000 {
		t.Fatalf("balance after buy-in = %d, want %d", bal, StartingBalance-4_000)
	}

	l.CashOut("p1", 4_500)
	if bal := l.Balance("p1"); bal != StartingBalance+500 {
		t.Fatalf("balance after cash-out = %d, want %d", bal, StartingBalance+500)
	}
}

func TestTopUpIfBroke(t *testing.T) {
	clock := newFakeClock(time.Now())
	l := NewLedger(NewMemoryStore(), clock.now)

	if err := l.BuyIn("p1", StartingBalance); err != nil {
		t.Fatalf("BuyIn failed: %v", err)
	}
	l.TopUpIfBroke("p1")
	if bal := l.Balance("p1"); bal != BankruptcyFloor {
		t.Fatalf("balance = %d, want BankruptcyFloor %d", bal, BankruptcyFloor)
	}
}

// TestConcurrentDebitCredit hammers a single ledger entry with concurrent
// Credit/Debit calls and asserts the final balance matches the expected net
// effect. Run with -race to catch any unsynchronized access.
func TestConcurrentDebitCredit(t *testing.T) {
	clock := newFakeClock(time.Now())
	l := NewLedger(NewMemoryStore(), clock.now)
	playerID := "racer"

	const goroutines = 100
	const creditAmt engine.Chips = 10
	const debitAmt engine.Chips = 10

	// Seed enough balance that debits never fail regardless of interleaving.
	l.Credit(playerID, engine.Chips(goroutines)*debitAmt)
	startBalance := l.Balance(playerID)

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			l.Credit(playerID, creditAmt)
		}()
		go func() {
			defer wg.Done()
			l.Debit(playerID, debitAmt)
		}()
	}
	wg.Wait()

	got := l.Balance(playerID)
	want := startBalance + engine.Chips(goroutines)*creditAmt - engine.Chips(goroutines)*debitAmt
	if got != want {
		t.Fatalf("final balance = %d, want %d", got, want)
	}
}
