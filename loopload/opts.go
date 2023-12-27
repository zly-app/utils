package loopload

import (
	"time"
)

type options struct {
	reloadTime time.Duration
}

func newOptions(opts []Option) *options {
	ret := &options{
		reloadTime: time.Minute,
	}
	for _, o := range opts {
		o(ret)
	}
	return ret
}

type Option func(opts *options)

func WithReloadTime(t time.Duration) Option {
	return func(opts *options) {
		opts.reloadTime = t
	}
}
