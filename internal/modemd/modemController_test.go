package modemd

import (
	"errors"
	"testing"
	"time"
)

// fakeSimChecker returns each response in order, repeating the last one once they run out.
type fakeSimChecker struct {
	statuses []string
	errs     []error
	calls    int
}

func (f *fakeSimChecker) check() (string, error) {
	i := f.calls
	if i >= len(f.statuses) {
		i = len(f.statuses) - 1
	}
	f.calls++
	return f.statuses[i], f.errs[i]
}

func TestWaitForSimCardReadyImmediately(t *testing.T) {
	f := &fakeSimChecker{statuses: []string{"READY"}, errs: []error{nil}}
	if !waitForSimCard(f.check, 50*time.Millisecond, 10*time.Millisecond) {
		t.Fatal("expected SIM card to be found")
	}
	if f.calls != 1 {
		t.Fatalf("expected 1 check, got %d", f.calls)
	}
}

func TestWaitForSimCardReadyAfterRetries(t *testing.T) {
	f := &fakeSimChecker{
		statuses: []string{"", "SIM not inserted", "READY"},
		errs:     []error{errors.New("+CME ERROR: SIM busy"), nil, nil},
	}
	if !waitForSimCard(f.check, 100*time.Millisecond, 10*time.Millisecond) {
		t.Fatal("expected SIM card to be found")
	}
	if f.calls != 3 {
		t.Fatalf("expected 3 checks, got %d", f.calls)
	}
}

func TestWaitForSimCardTimeout(t *testing.T) {
	f := &fakeSimChecker{statuses: []string{"SIM not inserted"}, errs: []error{nil}}
	start := time.Now()
	if waitForSimCard(f.check, 50*time.Millisecond, 10*time.Millisecond) {
		t.Fatal("expected SIM card not to be found")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("took %s, expected to give up after about 50ms", elapsed)
	}
	if f.calls < 2 {
		t.Fatalf("expected multiple checks before giving up, got %d", f.calls)
	}
}

func TestShouldBeOnRetriesAfterSimCardFailure(t *testing.T) {
	mc := ModemController{
		RetryInterval:        20 * time.Minute,
		RetrySimCardInterval: 2 * time.Hour,
		StartTime:            time.Now(),
		InitialOnDuration:    3 * time.Hour,
	}
	mc.failedToFindSimCard = true
	mc.lastFailedSimCard = time.Now()
	if on, reason := mc.shouldBeOnWithReason(); on {
		t.Fatalf("expected modem to stay off straight after SIM failure: %s", reason)
	}

	// Past the normal retry interval, but the SIM card retry interval is longer.
	mc.lastFailedSimCard = time.Now().Add(-21 * time.Minute)
	if on, reason := mc.shouldBeOnWithReason(); on {
		t.Fatalf("expected modem to stay off until the SIM card retry interval has passed: %s", reason)
	}

	mc.lastFailedSimCard = time.Now().Add(-121 * time.Minute)
	if on, reason := mc.shouldBeOnWithReason(); !on {
		t.Fatalf("expected modem to be allowed on after the retry interval: %s", reason)
	}
}

func TestShouldBeOnRetriesAfterFindModemFailure(t *testing.T) {
	mc := ModemController{
		RetryFindModemInterval: 3 * time.Hour,
		StartTime:              time.Now(),
		InitialOnDuration:      time.Hour,
	}
	mc.failedToFindModem = true
	mc.lastFailedFindModem = time.Now()
	if on, reason := mc.shouldBeOnWithReason(); on {
		t.Fatalf("expected modem to stay off straight after failing to find it: %s", reason)
	}

	mc.lastFailedFindModem = time.Now().Add(-4 * time.Hour)
	if on, reason := mc.shouldBeOnWithReason(); !on {
		t.Fatalf("expected modem to be allowed on after the retry interval: %s", reason)
	}
}
