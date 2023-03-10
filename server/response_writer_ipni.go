package server

import (
	"net/http"
	"path"
	"strings"

	"github.com/ipni/storetheindex/api/v0/finder/model"
	"github.com/multiformats/go-multihash"
)

var (
	_ lookupResponseWriter = (*ipniLookupResponseWriter)(nil)

	newline = []byte("\n")
)

type ipniLookupResponseWriter struct {
	jsonResponseWriter
	result model.MultihashResult
	count  int
}

func newIPNILookupResponseWriter(w http.ResponseWriter, preferJson bool) lookupResponseWriter {
	return &ipniLookupResponseWriter{
		jsonResponseWriter: newJsonResponseWriter(w, preferJson),
	}
}

func (i *ipniLookupResponseWriter) Accept(r *http.Request) error {
	if err := i.jsonResponseWriter.Accept(r); err != nil {
		return err
	}
	smh := strings.TrimPrefix(path.Base(r.URL.Path), "multihash/")
	var err error
	i.result.Multihash, err = multihash.FromB58String(smh)
	if err != nil {
		return errHttpResponse{message: err.Error(), status: http.StatusBadRequest}
	}
	return nil
}

func (i *ipniLookupResponseWriter) Key() multihash.Multihash {
	return i.result.Multihash
}

func (i *ipniLookupResponseWriter) WriteProviderResult(pr model.ProviderResult) error {
	if i.nd {
		if err := i.encoder.Encode(pr); err != nil {
			logger.Errorw("Failed to encode ndjson response", "err", err)
			return err
		}
		if _, err := i.w.Write(newline); err != nil {
			logger.Errorw("Failed to encode ndjson response", "err", err)
			return err
		}
		if i.f != nil {
			i.f.Flush()
		}
	} else {
		i.result.ProviderResults = append(i.result.ProviderResults, pr)
	}
	i.count++
	return nil
}

func (i *ipniLookupResponseWriter) Close() error {
	if i.count == 0 {
		return errHttpResponse{status: http.StatusNotFound}
	}
	logger.Debugw("Finished writing ipni results", "count", i.count)
	if i.nd {
		return nil
	}
	return i.encoder.Encode(i.result)
}
