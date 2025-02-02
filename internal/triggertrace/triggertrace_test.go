package triggertrace

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/traceroute-caller/hopannotation"
	"github.com/m-lab/traceroute-caller/internal/ipcache"
	"github.com/m-lab/traceroute-caller/parser"
	"github.com/m-lab/uuid-annotator/annotator"
)

var (
	forceTracerouteErr = "99.99.99.99" // force a failure running a traceroute
	forceParseErr      = "88.88.88.88" // force a failure parsing a traceroute output
	forceExtractErr    = "77.77.77.77" // force a failure extracting hops
	forceAnnotateErr   = "66.66.66.66" // force a failure annotating hops
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type fakeTracer struct {
	nTraces       int32
	nCachedTraces int32
}

func (ft *fakeTracer) Trace(remoteIP, cookie, uuid string, t time.Time) ([]byte, error) {
	defer func() { atomic.AddInt32(&ft.nTraces, 1) }()
	var jsonl string
	switch remoteIP {
	case forceTracerouteErr:
		return nil, errors.New("forced traceroute error")
	case forceParseErr:
		return []byte("forced parse error"), nil
	case forceExtractErr:
		jsonl = "./testdata/extract-error.jsonl"
	case forceAnnotateErr:
		jsonl = "./testdata/annotate-error.jsonl"
	default:
		jsonl = "./testdata/valid.jsonl"
	}
	content, err := ioutil.ReadFile(jsonl)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (ft *fakeTracer) CachedTrace(cookie, uuid string, t time.Time, cachedTest []byte) error {
	defer func() { atomic.AddInt32(&ft.nCachedTraces, 1) }()
	fmt.Printf("\nCachedTrace()\n")
	return nil
}

func (ft *fakeTracer) DontTrace() {
	log.Fatal("should not have called DontTrace()")
}

func (ft *fakeTracer) Traces() int32 {
	return atomic.LoadInt32(&ft.nTraces)
}

func (ft *fakeTracer) TracesCached() int32 {
	return atomic.LoadInt32(&ft.nCachedTraces)
}

type fakeAnnotator struct {
	nAnnotates int32
}

func (fa *fakeAnnotator) Annotate(ctx context.Context, ips []string) (map[string]*annotator.ClientAnnotations, error) {
	defer func() { atomic.AddInt32(&fa.nAnnotates, 1) }()
	annotations := make(map[string]*annotator.ClientAnnotations)
	for _, ip := range ips {
		if ip == forceAnnotateErr {
			return nil, errors.New("forced annotate error")
		}
		annotations[ip] = nil
	}
	return annotations, nil
}

func TestNewHandler(t *testing.T) {
	saveNetInterfaceAddrs := netInterfaceAddrs
	defer func() { netInterfaceAddrs = saveNetInterfaceAddrs }()

	netInterfaceAddrs = fakeInterfaceAddrsBad
	if _, err := newHandler(&fakeTracer{}); err == nil {
		t.Fatalf("NewHandler() = nil, want error")
	}

	netInterfaceAddrs = fakeInterfaceAddrs
	if _, err := newHandler(&fakeTracer{}); err != nil {
		t.Fatalf("NewHandler() = %v, want nil", err)
	}
}

func TestOpen(t *testing.T) {
	saveNetInterfaceAddrs := netInterfaceAddrs
	netInterfaceAddrs = fakeInterfaceAddrs
	defer func() { netInterfaceAddrs = saveNetInterfaceAddrs }()

	handler, err := newHandler(&fakeTracer{})
	if err != nil {
		t.Fatalf("NewHandler() = %v, want nil", err)
	}

	tests := []struct {
		name   string
		uuid   string
		sockID *inetdiag.SockID
	}{
		{"bad1", "", &inetdiag.SockID{SrcIP: "127.0.0.1", DstIP: "1.2.3.4"}}, // empty uuid
		{"bad1", "00001", nil},                                                     // nil sockID
		{"bad1", "00002", &inetdiag.SockID{SrcIP: "0.0.0.0"}},                      // DstIP empty
		{"bad1", "00003", &inetdiag.SockID{SrcIP: "invalid IP"}},                   // SrcIP invalid
		{"bad1", "00004", &inetdiag.SockID{SrcIP: "1.2.3.4", DstIP: "4.3.2.1"}},    // no local IP
		{"good1", "00005", &inetdiag.SockID{SrcIP: "127.0.0.1", DstIP: "1.2.3.4"}}, // SrcIP local
		{"good2", "00006", &inetdiag.SockID{SrcIP: "1.2.3.4", DstIP: "127.0.0.1"}}, // DstIP local
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler.Open(context.TODO(), time.Now(), test.uuid, test.sockID)
		})
	}
}

func TestClose(t *testing.T) {
	saveNetInterfaceAddrs := netInterfaceAddrs
	netInterfaceAddrs = fakeInterfaceAddrs
	defer func() { netInterfaceAddrs = saveNetInterfaceAddrs }()

	tests := []struct {
		name              string
		srcIP             string
		dstIP             string
		uuid              string
		callOpen          bool
		shouldWait        bool
		wantNTraces       int32
		wantNTracesCached int32
	}{
		{"bad1", "127.0.0.1", "1.2.3.4", "", true, false, 0, 0},
		{"bad2", "127.0.0.1", "2.3.4.5", "00001", false, false, 0, 0},
		{"bad3", "127.0.0.1", forceTracerouteErr, "00002", true, true, 1, 0},
		{"bad4", "127.0.0.1", forceParseErr, "00003", true, true, 1, 0},
		{"bad5", "127.0.0.1", forceExtractErr, "00004", true, true, 1, 0},
		{"bad6", "127.0.0.1", forceAnnotateErr, "00005", true, true, 1, 0},
		{"good1", "127.0.0.1", "3.4.5.6", "00006", true, true, 1, 0},
		{"good2", "4.5.6.7", "127.0.0.1", "00007", true, true, 1, 1},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tracer := &fakeTracer{}
			handler, err := newHandler(tracer)
			if err != nil {
				t.Fatalf("NewHandler() = %v, want nil", err)
			}
			if test.shouldWait {
				handler.done = make(chan struct{})
			}
			sockID := &inetdiag.SockID{SrcIP: test.srcIP, DstIP: test.dstIP}
			if test.callOpen {
				handler.Open(context.TODO(), time.Now(), test.uuid, sockID)
			}
			handler.Close(context.TODO(), time.Now(), test.uuid)
			if test.shouldWait {
				waitForTrace(t, handler)
			}
			if n := tracer.Traces(); n != test.wantNTraces {
				t.Fatalf("tracer.Traces() = %d, want %d", n, test.wantNTraces)
			}
			if n := tracer.TracesCached(); n != 0 {
				t.Fatalf("tracer.TracesCached() = %d, want 0", n)
			}
			// Should we do this again to make sure that the traceroute
			// is served from the cache?
			if test.wantNTracesCached > 0 {
				handler.done = make(chan struct{})
				handler.Open(context.TODO(), time.Now(), test.uuid, sockID)
				handler.Close(context.TODO(), time.Now(), test.uuid)
				waitForTrace(t, handler)
				if n := tracer.TracesCached(); n != test.wantNTracesCached {
					t.Fatalf("tracer.TracesCached() = %d, want %d", n, test.wantNTracesCached)
				}
			}
		})
	}
}

func newHandler(tracer *fakeTracer) (*Handler, error) {
	ipcCfg := ipcache.Config{
		EntryTimeout: 2 * time.Second,
		ScanPeriod:   1 * time.Second,
	}
	annotator := &fakeAnnotator{}
	haCfg := hopannotation.Config{
		AnnotatorClient: annotator,
		OutputPath:      "/tmp/annotation1",
	}
	newParser, err := parser.New("mda")
	if err != nil {
		return nil, err
	}
	return NewHandler(context.TODO(), tracer, ipcCfg, newParser, haCfg)
}

func waitForTrace(t *testing.T, handler *Handler) {
	t.Helper()
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for test to complete")
	case <-handler.done:
		handler.done = nil
	}
}

func fakeInterfaceAddrs() ([]net.Addr, error) {
	_, nw, _ := net.ParseCIDR("127.0.0.1/32")
	ip4, _ := net.ResolveIPAddr("ip4", "11.22.33.44")
	ip6, _ := net.ResolveIPAddr("ip6", "::1")
	return []net.Addr{
		nw,
		ip4,
		ip6,
	}, nil
}

func fakeInterfaceAddrsBad() ([]net.Addr, error) {
	return nil, errors.New("forced inet.InterfaceAddrs error")
}
