package a2s

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

type QueryResult struct {
	Reachable bool
	RTTMs     int64

	Name       string
	Game       string
	Map        string
	Version    string
	Players    uint8
	MaxPlayers uint8
	Bots       uint8

	ServerType  string
	Environment string

	Password bool
	VAC      bool

	Keywords string
}

const queryTimeout = 3 * time.Second

var a2sInfoRequest = []byte{
	0xFF, 0xFF, 0xFF, 0xFF,
	0x54,
	0x53, 0x6F, 0x75, 0x72, 0x63, 0x65, 0x20,
	0x45, 0x6E, 0x67, 0x69, 0x6E, 0x65, 0x20,
	0x51, 0x75, 0x65, 0x72, 0x79, 0x00,
}

type a2sInfo struct {
	Protocol    uint8
	Name        string
	Map         string
	Folder      string
	Game        string
	AppID       uint16
	Players     uint8
	MaxPlayers  uint8
	Bots        uint8
	ServerType  byte
	Environment byte
	Visibility  uint8
	VAC         uint8
	Version     string
	EDF         byte

	Port          *uint16
	SteamID       *uint64
	SpectatorPort *uint16
	SpectatorName *string
	Keywords      *string
	GameID        *uint64
}

func QueryInfo(host string, port int) (*QueryResult, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	conn, err := net.DialTimeout("udp", addr, queryTimeout)
	if err != nil {
		return &QueryResult{
			Reachable: false,
		}, fmt.Errorf("failed to open UDP connection: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(queryTimeout)); err != nil {
		return &QueryResult{
			Reachable: false,
		}, fmt.Errorf("failed to set UDP deadline: %w", err)
	}

	start := time.Now()

	raw, err := sendPacket(conn, a2sInfoRequest)
	if err != nil {
		return &QueryResult{
			Reachable: false,
		}, err
	}

	info, challenge, err := parseA2SInfoPacket(raw)
	if err != nil {
		return &QueryResult{
			Reachable: true,
		}, err
	}

	if challenge != nil {
		req := append([]byte{}, a2sInfoRequest...)
		req = append(req, challenge...)

		if err := conn.SetDeadline(time.Now().Add(queryTimeout)); err != nil {
			return &QueryResult{
				Reachable: true,
			}, fmt.Errorf("failed to reset UDP deadline: %w", err)
		}

		raw, err = sendPacket(conn, req)
		if err != nil {
			return &QueryResult{
				Reachable: true,
			}, fmt.Errorf("server requested challenge but retry failed: %w", err)
		}

		info, challenge, err = parseA2SInfoPacket(raw)
		if err != nil {
			return &QueryResult{
				Reachable: true,
			}, err
		}

		if challenge != nil {
			return &QueryResult{
				Reachable: true,
			}, fmt.Errorf("server returned an unexpected second challenge")
		}
	}

	if info == nil {
		return &QueryResult{
			Reachable: true,
		}, fmt.Errorf("server responded but no A2S_INFO data was parsed")
	}

	rtt := time.Since(start)

	return &QueryResult{
		Reachable:   true,
		RTTMs:       rtt.Milliseconds(),
		Name:        info.Name,
		Game:        info.Game,
		Map:         info.Map,
		Version:     info.Version,
		Players:     info.Players,
		MaxPlayers:  info.MaxPlayers,
		Bots:        info.Bots,
		ServerType:  mapServerType(info.ServerType),
		Environment: mapEnvironment(info.Environment),
		Password:    info.Visibility == 1,
		VAC:         info.VAC == 1,
		Keywords:    derefString(info.Keywords),
	}, nil
}

func sendPacket(conn net.Conn, packet []byte) ([]byte, error) {
	if _, err := conn.Write(packet); err != nil {
		return nil, fmt.Errorf("failed to send A2S_INFO request: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read UDP response: %w", err)
	}

	if n == 0 {
		return nil, fmt.Errorf("received empty UDP response")
	}

	return append([]byte(nil), buf[:n]...), nil
}

func parseA2SInfoPacket(packet []byte) (*a2sInfo, []byte, error) {
	if len(packet) < 5 {
		return nil, nil, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	r := bytes.NewReader(packet)

	var header int32
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, nil, fmt.Errorf("failed to read packet header: %w", err)
	}

	// Single-packet A2S response header should be 0xFFFFFFFF
	if header != -1 {
		return nil, nil, fmt.Errorf("unexpected packet header: %#x", uint32(header))
	}

	respType, err := r.ReadByte()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response type: %w", err)
	}

	switch respType {
	case 0x41:
		challenge := make([]byte, 4)
		if _, err := io.ReadFull(r, challenge); err != nil {
			return nil, nil, fmt.Errorf("failed to read challenge bytes: %w", err)
		}
		return nil, challenge, nil

	case 0x49:
		info := &a2sInfo{}

		if err := binary.Read(r, binary.LittleEndian, &info.Protocol); err != nil {
			return nil, nil, fmt.Errorf("failed to read protocol: %w", err)
		}

		if info.Name, err = readCString(r); err != nil {
			return nil, nil, fmt.Errorf("failed to read server name: %w", err)
		}
		if info.Map, err = readCString(r); err != nil {
			return nil, nil, fmt.Errorf("failed to read map: %w", err)
		}
		if info.Folder, err = readCString(r); err != nil {
			return nil, nil, fmt.Errorf("failed to read folder: %w", err)
		}
		if info.Game, err = readCString(r); err != nil {
			return nil, nil, fmt.Errorf("failed to read game: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.AppID); err != nil {
			return nil, nil, fmt.Errorf("failed to read app id: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.Players); err != nil {
			return nil, nil, fmt.Errorf("failed to read players: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.MaxPlayers); err != nil {
			return nil, nil, fmt.Errorf("failed to read max players: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.Bots); err != nil {
			return nil, nil, fmt.Errorf("failed to read bots: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.ServerType); err != nil {
			return nil, nil, fmt.Errorf("failed to read server type: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.Environment); err != nil {
			return nil, nil, fmt.Errorf("failed to read environment: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.Visibility); err != nil {
			return nil, nil, fmt.Errorf("failed to read visibility: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &info.VAC); err != nil {
			return nil, nil, fmt.Errorf("failed to read VAC: %w", err)
		}
		if info.Version, err = readCString(r); err != nil {
			return nil, nil, fmt.Errorf("failed to read version: %w", err)
		}

		// EDF is optional.
		edf, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return info, nil, nil
			}
			return nil, nil, fmt.Errorf("failed to read EDF: %w", err)
		}
		info.EDF = edf

		if edf&0x80 != 0 {
			var port uint16
			if err := binary.Read(r, binary.LittleEndian, &port); err != nil {
				return nil, nil, fmt.Errorf("failed to read query port: %w", err)
			}
			info.Port = &port
		}

		if edf&0x10 != 0 {
			var steamID uint64
			if err := binary.Read(r, binary.LittleEndian, &steamID); err != nil {
				return nil, nil, fmt.Errorf("failed to read steam id: %w", err)
			}
			info.SteamID = &steamID
		}

		if edf&0x40 != 0 {
			var spectatorPort uint16
			if err := binary.Read(r, binary.LittleEndian, &spectatorPort); err != nil {
				return nil, nil, fmt.Errorf("failed to read spectator port: %w", err)
			}

			spectatorName, err := readCString(r)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read spectator name: %w", err)
			}

			info.SpectatorPort = &spectatorPort
			info.SpectatorName = &spectatorName
		}

		if edf&0x20 != 0 {
			keywords, err := readCString(r)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read keywords: %w", err)
			}
			info.Keywords = &keywords
		}

		if edf&0x01 != 0 {
			var gameID uint64
			if err := binary.Read(r, binary.LittleEndian, &gameID); err != nil {
				return nil, nil, fmt.Errorf("failed to read game id: %w", err)
			}
			info.GameID = &gameID
		}

		return info, nil, nil

	default:
		return nil, nil, fmt.Errorf("unexpected response type: %#x", respType)
	}
}

func readCString(r *bytes.Reader) (string, error) {
	var out []byte

	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}

		if b == 0x00 {
			return string(out), nil
		}

		out = append(out, b)
	}
}

func mapServerType(b byte) string {
	switch b {
	case 'd':
		return "Dedicated"
	case 'l':
		return "Listen"
	case 'p':
		return "SourceTV"
	default:
		return string([]byte{b})
	}
}

func mapEnvironment(b byte) string {
	switch b {
	case 'l':
		return "Linux"
	case 'w':
		return "Windows"
	case 'm', 'o':
		return "macOS"
	default:
		return string([]byte{b})
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
