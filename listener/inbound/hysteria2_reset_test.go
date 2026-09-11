package inbound_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener/inbound"
	"github.com/stretchr/testify/require"
)

func TestHysteria2ResetProtectsPreVPNConnection(t *testing.T) {
	for _, mux := range []bool{false, true} {
		name := "plain"
		if mux {
			name = "smux"
		}
		t.Run(name, func(t *testing.T) { testHysteria2Reset(t, mux) })
	}
}

func testHysteria2Reset(t *testing.T, mux bool) {
	in, err := inbound.NewHysteria2(&inbound.Hysteria2Option{
		BaseOption: inbound.BaseOption{NameStr: "hy2-reset", Listen: "127.0.0.1", Port: "0"},
		Users:      map[string]string{"test": userUUID}, Certificate: tlsCertificate, PrivateKey: tlsPrivateKey,
	})
	require.NoError(t, err)
	testTunnel := NewHttpTestTunnel()
	t.Cleanup(func() { _ = testTunnel.Close() })
	require.NoError(t, in.Listen(testTunnel))
	t.Cleanup(func() { _ = in.Close() })
	addr, err := netip.ParseAddrPort(in.Address())
	require.NoError(t, err)
	options := map[string]any{
		"type": "hysteria2", "name": "pre-vpn-test", "server": addr.Addr().String(),
		"port": int(addr.Port()), "password": userUUID, "fingerprint": tlsFingerprint,
	}
	if mux {
		options["smux"] = map[string]any{"enabled": true, "protocol": "smux"}
	}
	proxy, err := adapter.ParseProxy(options)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proxy.Close() })

	request := func() {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return proxy.DialContext(ctx, &C.Metadata{NetWork: C.TCP, DstIP: remoteAddr, DstPort: 80})
			},
		}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + remoteAddr.String() + httpPath)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, httpData, body)
	}

	previousHook := dialer.DefaultSocketHook
	dialer.DefaultSocketHook = nil
	t.Cleanup(func() { dialer.DefaultSocketHook = previousHook })
	request() // A latency test before starting the VPN retains a QUIC socket.
	var protectedSockets atomic.Int32
	dialer.DefaultSocketHook = func(network, address string, conn syscall.RawConn) error {
		if strings.HasPrefix(network, "udp") {
			protectedSockets.Add(1)
		}
		return nil
	}
	for start := 1; start <= 2; start++ {
		require.NoError(t, outbound.ResetConnections(proxy.Adapter()))
		request()
		require.Equal(t, int32(start), protectedSockets.Load(), "VPN start must replace the preexisting UDP socket")
	}
	require.NoError(t, proxy.Close())
	require.NoError(t, outbound.ResetConnections(proxy.Adapter()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = proxy.DialContext(ctx, &C.Metadata{NetWork: C.TCP, DstIP: remoteAddr, DstPort: 80})
	require.Error(t, err, "a removed proxy must not be revived by a VPN restart")
}
