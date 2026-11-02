package slo

import (
	"errors"
	"math"
	"testing"
)

func TestCalculateAvailabilityBudgetWithinBudget(t *testing.T) {
	result, err := CalculateAvailabilityBudget(0.99, 9950, 50)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BudgetWithinBudget {
		t.Fatalf("status = %q, want within_budget", result.Status)
	}
	assertNear(t, *result.Availability, 0.995)
	assertNear(t, *result.AllowedBadEvents, 100)
	assertNear(t, *result.RemainingBadEvents, 50)
	assertNear(t, *result.BurnRate, 0.5)
}

func TestCalculateAvailabilityBudgetExhausted(t *testing.T) {
	result, err := CalculateAvailabilityBudget(0.99, 9800, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BudgetExhausted {
		t.Fatalf("status = %q, want exhausted", result.Status)
	}
	assertNear(t, *result.AllowedBadEvents, 100)
	assertNear(t, *result.RemainingBadEvents, 0)
	assertNear(t, *result.BudgetConsumedPercent, 200)
	assertNear(t, *result.BudgetRemainingPercent, 0)
	assertNear(t, *result.BurnRate, 2)
}

func TestCalculateAvailabilityBudgetInsufficientDataIsNotPerfectAvailability(t *testing.T) {
	result, err := CalculateAvailabilityBudget(0.999, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BudgetInsufficientData {
		t.Fatalf("status = %q, want insufficient_data", result.Status)
	}
	if result.Availability != nil || result.BurnRate != nil || result.AllowedBadEvents != nil {
		t.Fatalf("insufficient-data result unexpectedly contains calculated ratios: %+v", result)
	}
}

func TestCalculateAvailabilityBudgetZeroErrors(t *testing.T) {
	result, err := CalculateAvailabilityBudget(0.999, 10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BudgetWithinBudget {
		t.Fatalf("status = %q, want within_budget", result.Status)
	}
	assertNear(t, *result.BurnRate, 0)
	assertNear(t, *result.BudgetConsumedPercent, 0)
	assertNear(t, *result.BudgetRemainingPercent, 100)
}

func TestCalculateAvailabilityBudgetRejectsInvalidBoundaries(t *testing.T) {
	for _, input := range []struct {
		target float64
		good float64
		bad float64
	}{
		{target: 0.8999, good: 100, bad: 0},
		{target: 1, good: 100, bad: 0},
		{target: math.NaN(), good: 100, bad: 0},
		{target: math.Inf(1), good: 100, bad: 0},
		{target: 0.99, good: -1, bad: 0},
		{target: 0.99, good: 100, bad: -1},
		{target: 0.99, good: math.NaN(), bad: 0},
		{target: 0.99, good: math.Inf(1), bad: 0},
		{target: 0.99, good: math.MaxFloat64, bad: math.MaxFloat64},
	} {
		if _, err := CalculateAvailabilityBudget(input.target, input.good, input.bad); !errors.Is(err, ErrInvalidAvailabilityInput) {
			t.Errorf("input %+v error = %v, want ErrInvalidAvailabilityInput", input, err)
		}
	}
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value = %.12f, want %.12f", got, want)
	}
}
