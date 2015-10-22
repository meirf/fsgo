package report

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/foursquare/go-metrics"
)

func floatEquals(a, b float64) bool {
	return (a-b) < 0.000001 && (b-a) < 0.000001
}

func NewTestServer(t *testing.T, prefix string) (map[string]float64, net.Listener, *Recorder, *GraphiteConfig, *sync.WaitGroup) {
	res := make(map[string]float64)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("could not start dummy server:", err)
	}

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Fatal("dummy server error:", err)
			}
			r := bufio.NewReader(conn)
			line, err := r.ReadString('\n')
			for err == nil {
				parts := strings.Split(line, " ")
				i, _ := strconv.ParseFloat(parts[1], 0)
				if testing.Verbose() {
					t.Log("recv", parts[0], i)
				}
				res[parts[0]] = res[parts[0]] + i
				line, err = r.ReadString('\n')
			}
			wg.Done()
			conn.Close()
		}
	}()

	r := NewRecorder()
	r.Prefix = prefix

	c := GraphiteConfig{
		Addr:          ln.Addr().(*net.TCPAddr),
		FlushInterval: 10 * time.Millisecond,
	}

	return res, ln, r, &c, &wg
}

type DummyMeter struct {
	count int64
	rate1 float64
	metrics.Meter
}

func (m DummyMeter) Count() int64            { return m.count }
func (m DummyMeter) Rate1() float64          { return m.rate1 }
func (m DummyMeter) Snapshot() metrics.Meter { return m }

var _ (metrics.Meter) = (*DummyMeter)(nil)

func TestWrites(t *testing.T) {
	res, l, r, c, wg := NewTestServer(t, "foobar")
	defer l.Close()

	metrics.GetOrRegisterCounter("foo", r).Inc(2)

	r.Register("bar", DummyMeter{40, 4.0, metrics.NilMeter{}})

	metrics.GetOrRegisterTimer("baz", r).Update(time.Second * 5)
	metrics.GetOrRegisterTimer("baz", r).Update(time.Second * 4)
	metrics.GetOrRegisterTimer("baz", r).Update(time.Second * 3)
	metrics.GetOrRegisterTimer("baz", r).Update(time.Second * 2)
	metrics.GetOrRegisterTimer("baz", r).Update(time.Second * 1)

	wg.Add(1)
	r.Format = GoMetricsFormats
	sendToGraphite(r, c)
	wg.Wait()

	if expected, found := 2.0, res["foobar.foo.count"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 40.0, res["foobar.bar.count"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 4.0, res["foobar.bar.one-minute"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 5.0, res["foobar.baz.count"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if testing.Verbose() {
		for i, v := range res {
			t.Log("res", i, v)
		}
	}

	if expected, found := 5000.0, res["foobar.baz.99-percentile"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 3000.0, res["foobar.baz.50-percentile"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	r.Format = OstrichFormats

	for k, _ := range res {
		delete(res, k)
	}
	wg.Add(1)
	sendToGraphite(r, c)
	wg.Wait()

	if expected, found := 0.0, res["foobar.baz.99-percentile"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 0.0, res["foobar.baz.50-percentile"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if testing.Verbose() {
		for i, v := range res {
			t.Log("res", i, v)
		}
	}

	if expected, found := 5000.0, res["foobar.baz.percentiles.p99"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}

	if expected, found := 3000.0, res["foobar.baz.percentiles.p50"]; !floatEquals(found, expected) {
		t.Fatal("bad value:", expected, found)
	}
}