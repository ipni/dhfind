package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/ipni/go-libipni/apierror"
	finderhttpclient "github.com/ipni/go-libipni/find/client/http"
	"github.com/ischasny/dhfind/metrics"
	"go.uber.org/zap"
)

// preferJSON specifies weather to prefer JSON over NDJSON response when request accepts */*, i.e.
// any response format, has no `Accept` header at all.
const preferJSON = true

var (
	logger = logging.Logger("server")
)

type Server struct {
	s       *http.Server
	m       *metrics.Metrics
	dhaddr  string
	stiaddr string
	// simulation defines if the server is running in simulation mode. In simulation mode, the server
	// processs requests in a background worker pool and returns a 404 response immediately. The simulation mode is
	// used to tets performance of the server under load.
	simulation     bool
	simulationJobs chan simulationJob
	// simulationCancel is a cancel function that is used to cancel all background workers in simulation mode
	simulationCancel context.CancelFunc
	// simulationContext is a context that is used to cancel all background workers in simulation mode
	simulationContext context.Context
	// simulationWorkerCount is a number of background workers that find tasks are delegated to in simulation mode
	simulationWorkerCount int
}

// simulationJob is a job that is sent to a background worker in simulation mode. It's effectively a wrapper
// around http request and a flag that indicates if the request is for a multihash or a CID.
type simulationJob struct {
	request     *http.Request
	isMultihash bool
}

// responseWriterWithStatus is required to capture status code from ResponseWriter so that it can be reported
// to metrics in a unified way
type responseWriterWithStatus struct {
	http.ResponseWriter
	status int
	// TODO: remove that once the server is run in non-simulaiton mode
	header http.Header
}

func newResponseWriterWithStatus(w http.ResponseWriter) *responseWriterWithStatus {
	ws := &responseWriterWithStatus{
		ResponseWriter: w,
		// 200 status should be assumed by default if WriteHeader hasn't been called explicitly https://pkg.go.dev/net/http#ResponseWriter
		status: 200,
	}
	if w == nil {
		ws.header = make(http.Header)
	}
	return ws
}

func (rec *responseWriterWithStatus) Header() http.Header {
	if rec.ResponseWriter == nil {
		return rec.header
	}
	return rec.ResponseWriter.Header()
}

func (rec *responseWriterWithStatus) Write(b []byte) (int, error) {
	if rec.ResponseWriter == nil {
		return len(b), nil
	}
	return rec.ResponseWriter.Write(b)
}

func (rec *responseWriterWithStatus) WriteHeader(code int) {
	rec.status = code
	if rec.ResponseWriter != nil {
		rec.ResponseWriter.WriteHeader(code)
	}
}

func New(addr, dhaddr, stiaddr string, m *metrics.Metrics, simulation bool, simulationWorkerCount, simulationChannelSize int) (*Server, error) {
	var server Server

	server.s = &http.Server{
		Addr:    addr,
		Handler: server.serveMux(),
	}
	server.m = m
	server.dhaddr = dhaddr
	server.stiaddr = stiaddr
	server.simulation = simulation
	if simulation {
		server.simulationJobs = make(chan simulationJob, simulationChannelSize)
		server.simulationWorkerCount = simulationWorkerCount
	}

	return &server, nil
}

func (s *Server) serveMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/multihash/", s.handleMhSubtree)
	mux.HandleFunc("/cid/", s.handleCidSubtree)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/", s.handleCatchAll)
	return mux
}

func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.s.Addr)
	if err != nil {
		return err
	}
	go func() { _ = s.s.Serve(ln) }()

	if s.simulation {
		s.simulationContext, s.simulationCancel = context.WithCancel(ctx)
		for i := 1; i <= s.simulationWorkerCount; i++ {
			go s.simulationWorker()
		}
	}

	logger.Infow("Server started", "addr", ln.Addr())
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.simulation {
		s.simulationCancel()
	}
	return s.s.Shutdown(ctx)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ws := newResponseWriterWithStatus(w)
	defer s.reportLatency(start, ws.status, r.Method, "ready")
	discardBody(r)
	switch r.Method {
	case http.MethodGet:
		ws.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "", http.StatusNotFound)
	}
}

func (s *Server) handleCidSubtree(w http.ResponseWriter, r *http.Request) {
	s.handleFindSubtree(w, r, false)
}

func (s *Server) handleMhSubtree(w http.ResponseWriter, r *http.Request) {
	s.handleFindSubtree(w, r, true)
}

// handleFindSubtree is a handler for /multihash and /cid subtrees. isMultihash is true when /multihash subtree is requested.
func (s *Server) handleFindSubtree(w http.ResponseWriter, r *http.Request, isMultihash bool) {
	simulation := s.simulation
	if ss := r.URL.Query()["simulation"]; len(ss) > 0 {
		simulation = ss[0] == "true"
	}
	switch r.Method {
	case http.MethodGet:
		if simulation {
			select {
			case s.simulationJobs <- simulationJob{request: r, isMultihash: isMultihash}:
			default:
				logger.Info("Simulation channel full. Discarding value")
			}

			http.Error(w, "", http.StatusNotFound)
		} else {
			start := time.Now()
			ws := newResponseWriterWithStatus(w)
			defer s.reportLatency(start, ws.status, r.Method, "multihash")
			s.handleGetMh(newIPNILookupResponseWriter(ws, preferJSON, isMultihash), r)
		}
	default:
		discardBody(r)
		http.Error(w, "", http.StatusNotFound)
	}
}

func (s *Server) handleGetMh(w lookupResponseWriter, r *http.Request) {
	if err := w.Accept(r); err != nil {
		switch e := err.(type) {
		case errHttpResponse:
			e.WriteTo(w)
		default:
			logger.Errorw("Failed to accept lookup request", "err", err)
			http.Error(w, "", http.StatusInternalServerError)
		}
		return
	}
	mh := w.Key()
	log := logger.With("multihash", mh)
	ctx := context.Background()

	c, err := finderhttpclient.NewDHashClient(s.dhaddr, s.stiaddr)
	if err != nil {
		s.handleError(w, err, log)
		return
	}

	findResponse, err := c.Find(ctx, mh)

	if err != nil {
		s.handleError(w, err, log)
		return
	}

	if len(findResponse.MultihashResults) == 0 {
		log.Errorw("No multihash results")
		http.Error(w, "", http.StatusNotFound)
		return
	}

	mhr := findResponse.MultihashResults[0]

	for _, pr := range mhr.ProviderResults {
		if err := w.WriteProviderResult(pr); err != nil {
			log.Errorw("Failed to encode provider result", "err", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
	if err := w.Close(); err != nil {
		switch e := err.(type) {
		case errHttpResponse:
			e.WriteTo(w)
		default:
			log.Errorw("Failed to finalize lookup results", "err", err)
			http.Error(w, "", http.StatusInternalServerError)
		}
	}
}

func (s *Server) reportLatency(start time.Time, status int, method, path string) {
	s.m.RecordHttpLatency(context.Background(), time.Since(start), method, path, status)
}

func discardBody(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
}

func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	discardBody(r)
	http.Error(w, "", http.StatusNotFound)
}

// handleError writes an error to the response and logs it.
// A custom logger is passed in to propagate the request context with such parameters as multihash.
func (s *Server) handleError(w http.ResponseWriter, err error, log *zap.SugaredLogger) {
	var status int
	switch cerr := err.(type) {
	case ErrUnsupportedMulticodecCode, ErrMultihashDecode:
		status = http.StatusBadRequest
	case *apierror.Error:
		// TODO: do we need to treat metadata not founds differently to multihahs not found?
		status = cerr.Status()
	default:
		log.Warnw("Internal server error", "err", err)
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
}

func (s *Server) simulationWorker() {
	for {
		select {
		case <-s.simulationContext.Done():
			return
		case job := <-s.simulationJobs:
			var ws *responseWriterWithStatus
			start := time.Now()
			ws = newResponseWriterWithStatus(nil)
			s.handleGetMh(newIPNILookupResponseWriter(ws, preferJSON, job.isMultihash), job.request)
			s.reportLatency(start, ws.status, job.request.Method, "multihash")
		}
	}
}
