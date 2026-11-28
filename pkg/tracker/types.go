package tracker

import (
	"io"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/utils"
)

// FairnessTrackerBuilder helps configure and construct a FairnessTracker.
type FairnessTrackerBuilder struct {
	configuration *config.FairnessTrackerConfig
}

// NewFairnessTrackerBuilder returns a new builder pre-populated with the
// default configuration.
func NewFairnessTrackerBuilder() *FairnessTrackerBuilder {
	return &FairnessTrackerBuilder{
		configuration: config.DefaultFairnessTrackerConfig(),
	}
}

// Build constructs a tracker using the configuration accumulated on the builder.
// To build from a complete configuration instead, pass it to NewFairnessTracker.
func (bl *FairnessTrackerBuilder) Build() (*FairnessTracker, error) {
	return NewFairnessTracker(bl.configuration)
}

// SetL sets the number of levels used by the tracker.
func (bl *FairnessTrackerBuilder) SetL(L uint32) {
	bl.configuration.L = L
}

// SetM sets the number of buckets per level.
func (bl *FairnessTrackerBuilder) SetM(M uint32) {
	bl.configuration.M = M
}

// SetPd sets the decrement probability used on successful requests.
func (bl *FairnessTrackerBuilder) SetPd(Pd float64) {
	bl.configuration.Pd = Pd
}

// SetPi sets the increment probability used on failed requests.
func (bl *FairnessTrackerBuilder) SetPi(Pi float64) {
	bl.configuration.Pi = Pi
}

// SetLambda sets the decay rate for bucket probabilities. Zero disables decay;
// rotation still ends a penalty (see SetRotationFrequency).
func (bl *FairnessTrackerBuilder) SetLambda(Lambda float64) {
	bl.configuration.Lambda = Lambda
}

// SetIncludeStats indicates whether the tracker should return detailed stats.
func (bl *FairnessTrackerBuilder) SetIncludeStats(IncludeStats bool) {
	bl.configuration.IncludeStats = IncludeStats
}

// SetRotationFrequency configures how often the internal structures are rotated.
// A client that stays fully throttled is forgotten between one and two
// intervals after its last reported failure, whatever the decay rate is.
func (bl *FairnessTrackerBuilder) SetRotationFrequency(rotationFrequency time.Duration) {
	bl.configuration.RotationFrequency = rotationFrequency
}

// SetFinalProbabilityFunction sets the function used to derive the final
// throttling probability from all buckets.
func (bl *FairnessTrackerBuilder) SetFinalProbabilityFunction(finalProbabilityFunction config.FinalProbabilityFunction) {
	bl.configuration.FinalProbabilityFunction = finalProbabilityFunction
}

// SetHashKeySource sets the reader that supplies bucket placement keys. Leave
// it unset in production so keys come from crypto/rand; a simulation can pass
// a seeded reader to make placement reproducible.
func (bl *FairnessTrackerBuilder) SetHashKeySource(source io.Reader) {
	bl.configuration.HashKeySource = source
}

// SetDecisionSource sets the function that supplies the uniform random number
// in [0, 1) behind each throttling decision. Leave it unset in production; a
// simulation can pass a seeded source to make decisions reproducible.
func (bl *FairnessTrackerBuilder) SetDecisionSource(source func() float64) {
	bl.configuration.DecisionSource = source
}

// FairnessTrackerError is returned when the tracker encounters a recoverable
// error that should be surfaced to the caller.
type FairnessTrackerError struct {
	*utils.BaseError
}

// NewFairnessTrackerError creates a new FairnessTrackerError that wraps another
// error with additional context.
func NewFairnessTrackerError(wrapped error, msg string, args ...any) *FairnessTrackerError {
	return &FairnessTrackerError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
