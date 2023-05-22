package server

import (
	"net/http"
	"path"

	"github.com/multiformats/go-multihash"
)

const (
	shardKeyHeader = "x-ipni-dhstore-shard-key"
	metadataPath   = "metadata"
	multihashPath  = "multihash"
)

// ShardingRoundTripper adds x-ipni-dhstore-shard-key header to all GET requests for metadata and multihash.
type ShardingRoundTripper struct {
	http.RoundTripper
}

// NewShardingClient creates a new http.Client with ShardingRoundTripper transport. The client is designed to be used inside DHashClient for adding sharding headers to metadata and multihash lookups.
// Using the client for other purposes will not make any harm but will not bring any benefits either.
func NewShardingClient() *http.Client {
	return &http.Client{
		Transport: &ShardingRoundTripper{
			RoundTripper: http.DefaultTransport,
		},
	}
}

// RoundTrip adds x-ipni-dhstore-shard-key header to all GET requests for metadata and multihash.
// It follows the following rules:
//   - If this is not a GET request - do nothing;
//   - If the request path conforms to ".../metadata/XYZ" - then XYZ will be set as x-ipni-dhstore-shard-key header as-is;
//   - If the request path conforms to ".../multihash/XYZ" -  the last path of the URL will be treated as B58 encoded multihash.
func (cr *ShardingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet {
		var u, object, objectId, shardKey string
		u = r.URL.Path
		u, objectId = path.Split(u)
		// the remaining url should be at least as long as "/metadata/"
		if len(u) < len(metadataPath)+2 {
			goto DEFAULT
		}
		// don't forget to remove the last "/"
		_, object = path.Split(u[:len(u)-1])
		switch object {
		case metadataPath:
			shardKey = objectId
		case multihashPath:
			mh, err := multihash.FromB58String(objectId)
			if err != nil {
				goto DEFAULT
			}

			dmh, err := multihash.Decode(mh)
			// set the header only for double hashed lookups
			if err == nil && dmh.Code == multihash.DBL_SHA2_256 {
				shardKey = objectId
			}
		}

		if len(shardKey) > 0 {
			r.Header.Set(shardKeyHeader, shardKey)
		}
	}
DEFAULT:
	return cr.RoundTripper.RoundTrip(r)
}
