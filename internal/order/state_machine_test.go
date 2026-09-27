package order

import "testing"

func TestCanTransition_AllowedPaths(t *testing.T) {
	cases := []struct{ from, to Status }{
		{StatusPending, StatusWaitingPayment},
		{StatusPending, StatusCancelled},
		{StatusWaitingPayment, StatusPaid},
		{StatusWaitingPayment, StatusExpired},
		{StatusWaitingPayment, StatusCancelled},
		{StatusPaid, StatusProcessing},
		{StatusPaid, StatusCancelled},
		{StatusProcessing, StatusPacked},
		{StatusPacked, StatusShipped},
		{StatusShipped, StatusCompleted},
	}
	for _, c := range cases {
		if !CanTransition(c.from, c.to) {
			t.Errorf("expected %s -> %s to be allowed", c.from, c.to)
		}
	}
}

// TestCanTransition_ExplicitlyForbidden mereproduksi tepat 4 kasus yang spec
// (golang-enterprise-order-inventory-study-case.md Section 16) sebut secara
// eksplisit "tidak boleh".
func TestCanTransition_ExplicitlyForbidden(t *testing.T) {
	cases := []struct{ from, to Status }{
		{StatusCompleted, StatusPending},
		{StatusShipped, StatusPending},
		{StatusCancelled, StatusPaid},
		{StatusExpired, StatusShipped},
	}
	for _, c := range cases {
		if CanTransition(c.from, c.to) {
			t.Errorf("expected %s -> %s to be forbidden", c.from, c.to)
		}
	}
}

func TestCanTransition_UnknownFromState_ReturnsFalse(t *testing.T) {
	if CanTransition(Status("BOGUS"), StatusPaid) {
		t.Fatal("expected unknown from-state to never allow a transition")
	}
}

func TestCanTransition_CannotSkipStates(t *testing.T) {
	// PAID tidak boleh langsung ke SHIPPED, harus lewat PROCESSING->PACKED.
	if CanTransition(StatusPaid, StatusShipped) {
		t.Fatal("expected PAID -> SHIPPED to be forbidden (must go through PROCESSING/PACKED)")
	}
	// WAITING_PAYMENT tidak boleh langsung PROCESSING tanpa PAID dulu.
	if CanTransition(StatusWaitingPayment, StatusProcessing) {
		t.Fatal("expected WAITING_PAYMENT -> PROCESSING to be forbidden")
	}
}
