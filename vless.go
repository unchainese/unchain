package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
)

type ProtoVLESS struct {
	UserID      uuid.UUID
	DstProtocol string //tcp or udp
	dstHost     string
	dstHostType string //ipv6 or ipv4,domain
	dstPort     uint16
	Version     byte
	payload     []byte
}

const VLESS_VERSION = 0

func MakeVless(userID string, dstHost string, dstPort uint16, tcpOrUdp string, payload []byte) *ProtoVLESS {
	if tcpOrUdp != "tcp" && tcpOrUdp != "udp" {
		panic("tcpOrUdp must be tcp or udp")
	}
	return &ProtoVLESS{
		UserID:      uuid.MustParse(userID),
		DstProtocol: tcpOrUdp,
		dstHost:     dstHost,
		dstPort:     dstPort,
		Version:     VLESS_VERSION,
		payload:     payload,
	}
}

func (h ProtoVLESS) UUID() string {
	return h.UserID.String()
}
func (h ProtoVLESS) DataHeader() []byte {
	header := make([]byte, 0)
	header = append(header, h.Version)
	header = append(header, h.UserID[:]...)
	header = append(header, 0) //  no extra info length 0
	switch h.DstProtocol {
	case "tcp":
		header = append(header, 1)
	case "udp":
		header = append(header, 2)
	default:
		panic("unsupported protocol")
	}
	//two bytes of port
	header = append(header, byte(h.dstPort>>8), byte(h.dstPort&0xff))
	//address type
	thisIP := net.ParseIP(h.dstHost)
	if thisIP != nil && thisIP.To4() != nil {
		header = append(header, 1) // IPv4
		header = append(header, thisIP.To4()...)
	} else if thisIP != nil && thisIP.To16() != nil {
		header = append(header, 3) // IPv6
		header = append(header, thisIP.To16()...)
	} else {
		header = append(header, 2) // domain
		header = append(header, byte(len(h.dstHost)))
		header = append(header, []byte(h.dstHost)...)
	}
	header = append(header, h.payload...)
	return header
}

func (h ProtoVLESS) DataUdp() []byte {
	allData := make([]byte, 0)
	chunk := h.payload
	for index := 0; index < len(chunk); {
		if index+2 > len(chunk) {
			fmt.Println("Incomplete length buffer")
			return nil
		}
		lengthBuffer := chunk[index : index+2]
		udpPacketLength := binary.BigEndian.Uint16(lengthBuffer)
		if index+2+int(udpPacketLength) > len(chunk) {
			fmt.Println("Incomplete UDP packet")
			return nil
		}
		udpData := chunk[index+2 : index+2+int(udpPacketLength)]
		index = index + 2 + int(udpPacketLength)
		allData = append(allData, udpData...)
	}
	return allData
}

func (h ProtoVLESS) DataUdpWrong() []byte {
	allData := make([]byte, 0)
	chunk := h.payload
	for index := 0; index < len(chunk); {
		if index+2 > len(chunk) {
			fmt.Println("Incomplete length buffer")
			return nil
		}
		lengthBuffer := chunk[index : index+2]
		udpPacketLength := binary.BigEndian.Uint16(lengthBuffer)
		if index+2+int(udpPacketLength) > len(chunk) {
			fmt.Println("Incomplete UDP packet")
			return nil
		}
		udpData := chunk[index+2 : index+2+int(udpPacketLength)]
		index = index + 2 + int(udpPacketLength)
		allData = append(allData, udpData...)
	}
	return allData
}
func (h ProtoVLESS) DataTcp() []byte {
	return h.payload
}

func (h ProtoVLESS) AddrUdp() *net.UDPAddr {
	return &net.UDPAddr{IP: h.HostIP(), Port: int(h.dstPort)}
}
func (h ProtoVLESS) HostIP() net.IP {
	ip := net.ParseIP(h.dstHost)
	if ip == nil {
		ips, err := net.LookupIP(h.dstHost)
		if err != nil {
			h.Logger().Error("failed to resolve domain", "err", err.Error())
			return net.IPv4zero
		}
		if len(ips) == 0 {
			return net.IPv4zero
		}
		return ips[0]
	}
	return ip
}

func (h ProtoVLESS) HostPort() string {
	return net.JoinHostPort(h.dstHost, fmt.Sprintf("%d", h.dstPort))
}
func (h ProtoVLESS) Logger() *slog.Logger {
	return slog.With("userID", h.UserID.String(), "network", h.DstProtocol, "addr", h.HostPort())
}

// VLESSParse https://xtls.github.io/development/protocols/vless.html
func VLESSParse(buf []byte) (*ProtoVLESS, error) {
	payload := &ProtoVLESS{
		UserID:      uuid.Nil,
		DstProtocol: "",
		dstHost:     "",
		dstPort:     0,
		Version:     0,
		payload:     nil,
	}

	if len(buf) < 24 {
		return payload, errors.New("invalid payload length")
	}

	payload.Version = buf[0]
	payload.UserID = uuid.Must(uuid.FromBytes(buf[1:17]))
	extraInfoProtoBufLen := buf[17]

	command := buf[18+extraInfoProtoBufLen]
	switch command {
	case 1:
		payload.DstProtocol = "tcp"
	case 2:
		payload.DstProtocol = "udp"
	default:
		return payload, fmt.Errorf("command %d is not supported, command 01-tcp, 02-udp, 03-mux", command)
	}

	portIndex := 18 + extraInfoProtoBufLen + 1
	payload.dstPort = binary.BigEndian.Uint16(buf[portIndex : portIndex+2])

	addressIndex := portIndex + 2
	addressType := buf[addressIndex]
	addressValueIndex := addressIndex + 1

	switch addressType {
	case 1: // IPv4
		if len(buf) < int(addressValueIndex+net.IPv4len) {
			return nil, fmt.Errorf("invalid IPv4 address length")
		}
		payload.dstHost = net.IP(buf[addressValueIndex : addressValueIndex+net.IPv4len]).String()
		payload.payload = buf[addressValueIndex+net.IPv4len:]
		payload.dstHostType = "ipv4"
	case 2: // domain
		addressLength := buf[addressValueIndex]
		addressValueIndex++
		if len(buf) < int(addressValueIndex)+int(addressLength) {
			return nil, fmt.Errorf("invalid domain address length")
		}
		payload.dstHost = string(buf[addressValueIndex : int(addressValueIndex)+int(addressLength)])
		payload.payload = buf[int(addressValueIndex)+int(addressLength):]
		payload.dstHostType = "domain"

	case 3: // IPv6
		if len(buf) < int(addressValueIndex+net.IPv6len) {
			return nil, fmt.Errorf("invalid IPv6 address length")
		}
		payload.dstHost = net.IP(buf[addressValueIndex : addressValueIndex+net.IPv6len]).String()
		payload.payload = buf[addressValueIndex+net.IPv6len:]
		payload.dstHostType = "ipv6"
	default:
		return nil, fmt.Errorf("addressType %d is not supported", addressType)
	}

	return payload, nil
}
