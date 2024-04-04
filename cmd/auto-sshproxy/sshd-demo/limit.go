package main

import (
	"context"
	"github.com/juju/ratelimit"
	"golang.org/x/crypto/ssh"
)

type rateLimitedChannelWrapper struct {
	ssh.Channel
	bucket *ratelimit.Bucket
	ctx    context.Context
}

// Read data from a connection. The reads are rate limited at bytes per second.
// len(b) must be bigger than the burst size set on the limiter, otherwise an error is returned.
func (c *rateLimitedChannelWrapper) Read(b []byte) (int, error) {
	return ratelimit.Reader(c.Channel, c.bucket).Read(b)
}

// Write data to a connection. The writes are rate limited at bytes per second.
// len(b) must be bigger than the burst size set on the limiter, otherwise an error is returned.
func (c *rateLimitedChannelWrapper) Write(b []byte) (int, error) {
	return ratelimit.Writer(c.Channel, c.bucket).Write(b)

}

// NewRateLimitedChannel returns a ssh.Channel that has its Read method rate limited
// by the limiter.
func NewRateLimitedChannel(ctx context.Context, bucket *ratelimit.Bucket, ch ssh.Channel) ssh.Channel {
	return &rateLimitedChannelWrapper{
		Channel: ch,
		ctx:     ctx,
		bucket:  bucket,
	}
}
