package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShardingClient(t *testing.T) {
	var seenMultihash, seenInvalidMultihash, seenMetadata bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Path
		if r.Method != http.MethodGet {
			require.Equal(t, "", r.Header.Get(shardKeyHeader))
			return
		}
		if strings.Contains(u, "multihash/2wvrrzCXz5kN4bEKCNvZgPECjiNwdXLeHuqU5yZzeRN7j8o") {
			require.Equal(t, "2wvrrzCXz5kN4bEKCNvZgPECjiNwdXLeHuqU5yZzeRN7j8o", r.Header.Get(shardKeyHeader))
			seenMultihash = true
		} else if strings.Contains(u, "multihash/INVALID") {
			require.Equal(t, "INVALID", r.Header.Get(shardKeyHeader))
			seenInvalidMultihash = true
		} else if strings.Contains(u, "metadata") {
			require.Equal(t, "ABCD", r.Header.Get(shardKeyHeader))
			seenMetadata = true
		} else {
			require.Equal(t, "", r.Header.Get(shardKeyHeader))
		}
	}))

	c := newShardingClient()

	sendRequest(t, c, server.URL+"/multihash/2wvrrzCXz5kN4bEKCNvZgPECjiNwdXLeHuqU5yZzeRN7j8o", http.MethodGet) // double hashed multihash
	sendRequest(t, c, server.URL+"/multihash/INVALID", http.MethodGet)                                         // invalid multihash - should still succeed
	sendRequest(t, c, server.URL+"/metadata/ABCD", http.MethodGet)
	sendRequest(t, c, server.URL+"/someOtherPath/ABCD", http.MethodGet)
	sendRequest(t, c, server.URL+"/someOtherPath/ABCD", http.MethodPost)
	require.True(t, seenMetadata && seenMultihash && seenInvalidMultihash)
}

func sendRequest(t *testing.T, c *http.Client, u, method string) {
	req, err := http.NewRequestWithContext(context.Background(), method, u, nil)
	require.NoError(t, err)
	_, err = c.Do(req)
	require.NoError(t, err)
}
