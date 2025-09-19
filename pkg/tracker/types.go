package tracker

import (
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

// BuildWithDefaultConfig builds a tracker using DefaultFairnessTrackerConfig.
func (bl *FairnessTrackerBuilder) BuildWithDefaultConfig() (*FairnessTracker, error) {
	return NewFairnessTracker(config.DefaultFairnessTrackerConfig())
}

// BuildWithConfig builds a tracker using the supplied configuration.
func (bl *FairnessTrackerBuilder) BuildWithConfig(configuration *config.FairnessTrackerConfig) (*FairnessTracker, error) {
	if configuration == nil {
		return nil, NewFairnessTrackerError(nil, "Configuration cannot be nil")
	}
	return NewFairnessTracker(configuration)
}

// Build constructs a tracker using the configuration accumulated on the builder.
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

// SetLambda sets the decay rate for bucket probabilities.
func (bl *FairnessTrackerBuilder) SetLambda(Lambda float64) {
	bl.configuration.Lambda = Lambda
}

// SetIncludeStats indicates whether the tracker should return detailed stats.
func (bl *FairnessTrackerBuilder) SetIncludeStats(IncludeStats bool) {
	bl.configuration.IncludeStats = IncludeStats
}

// SetRotationFrequency configures how often the internal structures are rotated.
func (bl *FairnessTrackerBuilder) SetRotationFrequency(rotationFrequency time.Duration) {
	bl.configuration.RotationFrequency = rotationFrequency
}

// SetFinalProbabilityFunction sets the function used to derive the final
// throttling probability from all buckets.
func (bl *FairnessTrackerBuilder) SetFinalProbabilityFunction(finalProbabilityFunction config.FinalProbabilityFunction) {
	bl.configuration.FinalProbabilityFunction = finalProbabilityFunction
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
