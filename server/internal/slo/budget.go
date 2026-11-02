package slo

import (
	"errors"
	"math"
)

const (
	MinimumAvailabilityTarget = 0.90
	MaximumAvailabilityTarget = 0.9999
)

var ErrInvalidAvailabilityInput = errors.New("invalid availability SLO input")

type BudgetStatus string

const (
	BudgetInsufficientData BudgetStatus = "insufficient_data"
	BudgetWithinBudget     BudgetStatus = "within_budget"
	BudgetExhausted        BudgetStatus = "exhausted"
)

type AvailabilityBudget struct {
	Status                   BudgetStatus `json:"status"`
	Target                   float64      `json:"target"`
	GoodEvents               float64      `json:"good_events"`
	BadEvents                float64      `json:"bad_events"`
	TotalEvents              float64      `json:"total_events"`
	Availability             *float64     `json:"availability,omitempty"`
	AllowedBadEvents         *float64     `json:"allowed_bad_events,omitempty"`
	RemainingBadEvents       *float64     `json:"remaining_bad_events,omitempty"`
	BudgetConsumedPercent    *float64     `json:"budget_consumed_percent,omitempty"`
	BudgetRemainingPercent   *float64     `json:"budget_remaining_percent,omitempty"`
	BurnRate                 *float64     `json:"burn_rate,omitempty"`
}

// CalculateAvailabilityBudget uses only the supplied good/bad valid events.
// A lack of observations is explicitly insufficient_data, never 100% uptime.
// The burn rate is observed error ratio divided by the allowed error ratio.
func CalculateAvailabilityBudget(target, goodEvents, badEvents float64) (AvailabilityBudget, error) {
	if math.IsNaN(target) || math.IsInf(target, 0) ||
		target < MinimumAvailabilityTarget || target > MaximumAvailabilityTarget ||
		math.IsNaN(goodEvents) || math.IsInf(goodEvents, 0) || goodEvents < 0 ||
		math.IsNaN(badEvents) || math.IsInf(badEvents, 0) || badEvents < 0 {
		return AvailabilityBudget{}, ErrInvalidAvailabilityInput
	}

	total := goodEvents + badEvents
	if math.IsNaN(total) || math.IsInf(total, 0) {
		return AvailabilityBudget{}, ErrInvalidAvailabilityInput
	}
	result := AvailabilityBudget{
		Status: BudgetInsufficientData,
		Target: target,
		GoodEvents: goodEvents,
		BadEvents: badEvents,
		TotalEvents: total,
	}
	if total == 0 {
		return result, nil
	}

	availability := goodEvents / total
	allowedBad := total * (1 - target)
	if allowedBad <= 0 || math.IsNaN(allowedBad) || math.IsInf(allowedBad, 0) {
		return AvailabilityBudget{}, ErrInvalidAvailabilityInput
	}
	remainingBad := math.Max(0, allowedBad-badEvents)
	burnRate := badEvents / allowedBad
	consumedPercent := burnRate * 100
	remainingPercent := math.Max(0, 100-consumedPercent)

	result.Availability = floatPointer(availability)
	result.AllowedBadEvents = floatPointer(allowedBad)
	result.RemainingBadEvents = floatPointer(remainingBad)
	result.BudgetConsumedPercent = floatPointer(consumedPercent)
	result.BudgetRemainingPercent = floatPointer(remainingPercent)
	result.BurnRate = floatPointer(burnRate)
	if remainingBad > 0 {
		result.Status = BudgetWithinBudget
	} else {
		result.Status = BudgetExhausted
	}
	return result, nil
}

func floatPointer(value float64) *float64 {
	return &value
}
