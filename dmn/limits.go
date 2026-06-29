package dmn

import (
	"time"

	"github.com/pblumer/temis/internal/feel"
)

// Limits bounds the resources a single compilation or evaluation may consume,
// turning hostile input (deep recursion, runaway comprehensions, huge lists,
// pathological models) into a clean error instead of a hang or out-of-memory
// (ADR-0008). A zero field falls back to the built-in default for that
// dimension, so a caller may tighten one limit without restating the rest.
type Limits struct {
	MaxCallDepth   int           // nested user-function (BKM / function literal) calls
	MaxIterations  int           // total iteration steps across all comprehensions in one evaluation
	MaxListSize    int           // element count of any single list produced by a comprehension
	CompileTimeout time.Duration // wall-clock budget for Compile when the context has no earlier deadline
}

// WithLimits sets the engine's resource limits. Unset (zero) fields keep their
// defaults; see Limits.
func WithLimits(l Limits) Option {
	return func(c *config) {
		c.limits = l
		c.limitsSet = true
	}
}

// feelLimits resolves the configured limits into the evaluation limits, filling
// unset fields from the defaults.
func (c config) feelLimits() feel.Limits {
	def := feel.DefaultLimits()
	lim := feel.Limits{
		MaxCallDepth:  def.MaxCallDepth,
		MaxIterations: def.MaxIterations,
		MaxListSize:   def.MaxListSize,
	}
	if c.limits.MaxCallDepth > 0 {
		lim.MaxCallDepth = c.limits.MaxCallDepth
	}
	if c.limits.MaxIterations > 0 {
		lim.MaxIterations = c.limits.MaxIterations
	}
	if c.limits.MaxListSize > 0 {
		lim.MaxListSize = c.limits.MaxListSize
	}
	return lim
}
