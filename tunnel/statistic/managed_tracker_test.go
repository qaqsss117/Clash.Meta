package statistic

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

type meteredTestConn struct {
	C.Conn
	socket net.Conn
	tag    string
}

func (c *meteredTestConn) Read(p []byte) (int, error)  { return c.socket.Read(p) }
func (c *meteredTestConn) Write(p []byte) (int, error) { return c.socket.Write(p) }
func (c *meteredTestConn) Close() error                { return c.socket.Close() }
func (c *meteredTestConn) Chains() C.Chain             { return C.Chain{c.tag, "automatic", "selector"} }
func (c *meteredTestConn) RemoteDestination() string   { return "test" }

type meteredTestPacket struct {
	C.PacketConn
	socket net.PacketConn
	tag    string
}

func (c *meteredTestPacket) ReadFrom(p []byte) (int, net.Addr, error)  { return c.socket.ReadFrom(p) }
func (c *meteredTestPacket) WriteTo(p []byte, a net.Addr) (int, error) { return c.socket.WriteTo(p, a) }
func (c *meteredTestPacket) Close() error                              { return c.socket.Close() }
func (c *meteredTestPacket) Chains() C.Chain                           { return C.Chain{c.tag, "automatic"} }
func (c *meteredTestPacket) RemoteDestination() string                 { return "test" }

func TestManagedShortTCPAndUDPAreCountedWithoutPollingConnections(t *testing.T) {
	Managed.Begin(map[string]string{"upstream-7": "7"}, 10000, time.Minute)
	defer Managed.Require(false)
	for _, tag := range []string{"upstream-7", "DIRECT", "self-hosted"} {
		local, peer := net.Pipe()
		tracker := NewTCPTracker(&meteredTestConn{socket: local, tag: tag}, DefaultManager, &C.Metadata{}, nil, 0, 0, true)
		go func() {
			defer peer.Close()
			b := make([]byte, 3)
			_, _ = io.ReadFull(peer, b)
			_, _ = peer.Write([]byte("reply"))
		}()
		if _, err := tracker.Write([]byte("ask")); err != nil {
			t.Fatal(err)
		}
		received, err := io.ReadAll(tracker)
		if err != nil || !bytes.Equal(received, []byte("reply")) {
			t.Fatalf("read: %q %v", received, err)
		}
		_ = tracker.Close()
	}
	a, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_ = a.SetDeadline(time.Now().Add(time.Second))
	_ = b.SetDeadline(time.Now().Add(time.Second))
	udp := NewUDPTracker(&meteredTestPacket{socket: a, tag: "upstream-7"}, DefaultManager, &C.Metadata{}, nil, 0, 0, true)
	if _, err = udp.WriteTo([]byte("hello"), b.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	if _, _, err = b.ReadFrom(buf); err != nil {
		t.Fatal(err)
	}
	if _, err = b.WriteTo([]byte("world!"), a.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = udp.ReadFrom(buf); err != nil {
		t.Fatal(err)
	}
	_ = udp.Close()
	totals, _ := Managed.Snapshot()
	if totals["7"] != [2]int64{8, 11} || len(totals) != 1 {
		t.Fatalf("incorrect actual stream counters: %v", totals)
	}
}

func TestManagedOldConnectionCannotContaminateRestartedSession(t *testing.T) {
	Managed.Begin(map[string]string{"upstream-7": "7"}, 10000, time.Minute)
	defer Managed.Require(false)
	old := Managed.Generation()
	Managed.Begin(map[string]string{"upstream-7": "7"}, 10000, time.Minute)
	Managed.AddGeneration(old, "upstream-7", 0, 500)
	totals, _ := Managed.Snapshot()
	if len(totals) != 0 {
		t.Fatal("previous generation entered new ledger")
	}
}
