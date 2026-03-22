package a2s

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

type ProbeResult struct {
	PacketSent bool
	Responded  bool
	RTTMs      int64
	BytesRead  int
	Status     string
}

func ProbeUDP(host string, port int) (*ProbeResult, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	conn, err := net.DialTimeout("udp", addr, queryTimeout)
	if err != nil {
		return &ProbeResult{
			PacketSent: false,
			Responded:  false,
			Status:     "send_failed",
		}, fmt.Errorf("failed to open UDP connection: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(queryTimeout)); err != nil {
		return &ProbeResult{
			PacketSent: false,
			Responded:  false,
			Status:     "send_failed",
		}, fmt.Errorf("failed to set UDP deadline: %w", err)
	}

	start := time.Now()

	payload := []byte("mimic-lens-probe")

	if _, err := conn.Write(payload); err != nil {
		return &ProbeResult{
			PacketSent: false,
			Responded:  false,
			Status:     "send_failed",
		}, fmt.Errorf("failed to send UDP probe: %w", err)
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		// timeout / no response is not really a hard app error
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return &ProbeResult{
				PacketSent: true,
				Responded:  false,
				RTTMs:      time.Since(start).Milliseconds(),
				BytesRead:  0,
				Status:     "no_response",
			}, nil
		}

		return &ProbeResult{
			PacketSent: true,
			Responded:  false,
			RTTMs:      time.Since(start).Milliseconds(),
			BytesRead:  0,
			Status:     "port_unreachable",
		}, fmt.Errorf("probe error: %w", err)
	}

	return &ProbeResult{
		PacketSent: true,
		Responded:  true,
		RTTMs:      time.Since(start).Milliseconds(),
		BytesRead:  n,
		Status:     "response_received",
	}, nil
}
