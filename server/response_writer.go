package server

import (
	"io"
	"net/http"

	"github.com/ipni/storetheindex/api/v0/finder/model"
	"github.com/multiformats/go-multihash"
)

type (
	selectiveResponseWriter interface {
		http.ResponseWriter
		Accept(r *http.Request) error
	}
	lookupResponseWriter interface {
		io.Closer
		selectiveResponseWriter
		Key() multihash.Multihash
		WriteProviderResult(model.ProviderResult) error
	}
)
