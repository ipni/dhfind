package server

import (
	"net/http"
	"strings"
)

const (
	shardKeyHeader = "x-ipni-dhstore-shard-key"
)

// shardingRoundTripper adds x-ipni-dhstore-shard-key header to all GET requests for metadata and multihash.
type shardingRoundTripper struct {
	http.RoundTripper
}

// newShardingClient creates a new http.Client with ShardingRoundTripper transport. The client is designed to be used inside DHashClient for adding sharding headers to metadata and multihash lookups.
// Using the client for other purposes will not make any harm but will not bring any benefits either.
func newShardingClient() *http.Client {
	return &http.Client{
		Transport: &shardingRoundTripper{
			RoundTripper: http.DefaultTransport,
		},
	}
}

// RoundTrip adds x-ipni-dhstore-shard-key header to all GET requests for metadata and multihash.
// It follows the following rules:
//   - If this is not a GET request - do nothing;
//   - If the request path conforms to ".../metadata/XYZ" - then XYZ will be set as x-ipni-dhstore-shard-key header as-is;
//   - If the request path conforms to ".../multihash/XYZ" -  the last path of the URL will be treated as B58 encoded multihash.
func (cr *shardingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet {
		const (
			metadataPath  = "/metadata/"
			multihashPath = "/multihash/"
		)
		switch {
		case strings.HasPrefix(r.URL.Path, metadataPath):
			r.Header.Set(shardKeyHeader, strings.TrimPrefix(r.URL.Path, metadataPath))
		case strings.HasPrefix(r.URL.Path, multihashPath):
			// Considering...
			// - dhfind is only used internally
			// - has "dh" in its name, i.e. double-hashed,
			// - and the this roundtripper is not in the public client library anymore, i.e. moved here
			// ...do not bother checking if suffix is a valid multihash with DBL_SHA2_256 code.
			// Because, the chances are the lookup will not find anything anyway and
			// the presence of shard key would make no difference.
			r.Header.Set(shardKeyHeader, strings.TrimPrefix(r.URL.Path, multihashPath))
		}
	}
	return cr.RoundTripper.RoundTrip(r)
}
