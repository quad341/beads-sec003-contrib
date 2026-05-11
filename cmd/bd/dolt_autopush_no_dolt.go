//go:build no_dolt

package main

import "context"

// maybeAutoPush is a no-op under no_dolt — there is no Dolt remote to push to.
func maybeAutoPush(_ context.Context) {}
