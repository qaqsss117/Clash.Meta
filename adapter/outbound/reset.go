package outbound

import C "github.com/metacubex/mihomo/constant"

// ResetConnections retires reusable transports after a socket routing change.
func ResetConnections(proxy C.ProxyAdapter) error {
	switch p := proxy.(type) {
	case *autoCloseProxyAdapter:
		return ResetConnections(p.ProxyAdapter)
	case *SingMux:
		return ResetConnections(p.ProxyAdapter)
	case *Hysteria2:
		if p.client != nil {
			// A nil error retires the QUIC session without permanently closing
			// the client, so its next request opens a newly protected UDP socket.
			return p.client.CloseWithError(nil)
		}
	}
	return nil
}
