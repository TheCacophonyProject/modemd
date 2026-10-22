package modemd

import (
	"errors"
	"os"
	"path/filepath"
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

func TestShouldBeOnGivesUpAfterSimCardFailsInARow(t *testing.T) {
	mc := ModemController{
		RetrySimCardInterval: 2 * time.Hour,
		StartTime:            time.Now(),
		InitialOnDuration:    24 * time.Hour,
	}
	mc.failedToFindSimCard = true
	mc.lastFailedSimCard = time.Now().Add(-3 * time.Hour)

	mc.simCardFailsInARow = maxSimCardFailsInARow - 1
	if on, reason := mc.shouldBeOnWithReason(); !on {
		t.Fatalf("expected modem to retry after the retry interval before giving up: %s", reason)
	}

	mc.simCardFailsInARow = maxSimCardFailsInARow
	if on, reason := mc.shouldBeOnWithReason(); on {
		t.Fatalf("expected modem to stay off after failing %d times in a row: %s", maxSimCardFailsInARow, reason)
	}
	if !mc.waitingToRetrySimCard() {
		t.Fatal("expected to keep waiting to retry the SIM card after giving up")
	}

	// A stay-on request still turns the modem on, but the SIM card isn't checked again.
	mc.stayOnUntil = time.Now().Add(time.Hour)
	if on, reason := mc.shouldBeOnWithReason(); !on {
		t.Fatalf("expected a stay-on request to override giving up: %s", reason)
	}
	if !mc.waitingToRetrySimCard() {
		t.Fatal("expected not to retry the SIM card during a stay-on request after giving up")
	}
}

func TestSimCardFailsInARowSavedAndLoaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modemd", "sim-card-fails")
	if fails := loadSimCardFailsInARow(path); fails != 0 {
		t.Fatalf("expected 0 when the file doesn't exist, got %d", fails)
	}

	saveSimCardFailsInARow(path, maxSimCardFailsInARow)
	if fails := loadSimCardFailsInARow(path); fails != maxSimCardFailsInARow {
		t.Fatalf("expected %d, got %d", maxSimCardFailsInARow, fails)
	}

	if err := os.WriteFile(path, []byte("not a number"), 0644); err != nil {
		t.Fatal(err)
	}
	if fails := loadSimCardFailsInARow(path); fails != 0 {
		t.Fatalf("expected 0 for an invalid file, got %d", fails)
	}
}

func TestSimCardRetriedOnceAfterRestartWhenGaveUp(t *testing.T) {
	// After a restart the saved count is loaded, but nothing has failed yet this run.
	mc := ModemController{RetrySimCardInterval: 2 * time.Hour}
	mc.simCardFailsInARow = maxSimCardFailsInARow
	if mc.waitingToRetrySimCard() {
		t.Fatal("expected the SIM card to be checked once after a restart")
	}
	if attempts := mc.simCardPowerOnAttemptsAllowed(); attempts != 1 {
		t.Fatalf("expected 1 power on attempt after giving up, got %d", attempts)
	}

	mc.simCardFailsInARow = maxSimCardFailsInARow - 1
	if attempts := mc.simCardPowerOnAttemptsAllowed(); attempts != simCardPowerOnAttempts {
		t.Fatalf("expected %d power on attempts before giving up, got %d", simCardPowerOnAttempts, attempts)
	}
	mc.simCardFailsInARow = maxSimCardFailsInARow

	// One more failure and it gives up again straight away.
	mc.failedToFindSimCard = true
	mc.lastFailedSimCard = time.Now().Add(-3 * time.Hour)
	mc.simCardFailsInARow++
	if !mc.waitingToRetrySimCard() {
		t.Fatal("expected to give up again after failing once after a restart")
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
