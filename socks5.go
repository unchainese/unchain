package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

// SOCKS5 protocol and server constants
const (
	socksVersion5    = 0x05
	authMethodNoAuth = 0x00
	reservedField    = 0x00

	cmdConnect      = 0x01
	cmdUDPAssociate = 0x03

	addrTypeIPv4   = 0x01
	addrTypeDomain = 0x03
	addrTypeIPv6   = 0x04

	replySucceeded       = 0x00
	replyHostUnreachable = 0x04

	responseFixedLen = 10
	udpHeaderLen     = 10
	maxUDPPacketSize = 65536

	networkTCP       = "tcp"
	networkUDP       = "udp"
	localhostAnyPort = "127.0.0.1:0"
)

var (
	socks5Host string = "127.0.0.1"
	socks5Port int    = 1088
	vlessUUID         = "13a1b3b8-3c1c-4335-868a-396534d2317b"
	wsURL             = "ws://aws.libragen.cn/wsv/v1?ed=2560"
)

func StartSocks5Server() {
	addr := fmt.Sprintf("%s:%d", socks5Host, socks5Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to start SOCKS5 server: %v", err))
		os.Exit(1)
	}
	defer listener.Close()

	slog.Info(fmt.Sprintf("SOCKS5 server started on %s", addr))
	slog.Info("Press Ctrl+C to stop the server")

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}
		go handleSocks5Connection(conn)
	}
}

func handleSocks5Connection(client net.Conn) {
	defer client.Close()
	slog.Debug(fmt.Sprintf("New connection from %s", client.RemoteAddr()))

	// Step 1: Version and authentication methods
	if err := handleHandshake(client); err != nil {
		slog.Error(fmt.Sprintf("Handshake failed: %v", err))
		return
	}

	// Step 2: Request details
	request, err := handleRequest(client)
	if err != nil {
		slog.Error(fmt.Sprintf("Request handling failed: %v", err))
		return
	}

	// Step 3: Connect to target and relay data
	if err := handleRelay(client, request); err != nil {
		slog.Error(fmt.Sprintf("Relay failed: %v", err))
		return
	}
}

func handleHandshake(client net.Conn) error {
	// Read version and number of authentication methods
	buf := make([]byte, 2)
	if _, err := io.ReadFull(client, buf); err != nil {
		return fmt.Errorf("failed to read handshake: %v", err)
	}

	version := buf[0]
	if version != socksVersion5 {
		return fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	numMethods := buf[1]
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(client, methods); err != nil {
		return fmt.Errorf("failed to read authentication methods: %v", err)
	}

	// For simplicity, we'll accept any authentication method (including no authentication)
	// In production, you might want to implement proper authentication
	response := []byte{socksVersion5, authMethodNoAuth} // Version 5, No authentication required
	if _, err := client.Write(response); err != nil {
		return fmt.Errorf("failed to write handshake response: %v", err)
	}

	return nil
}

type socks5Request struct {
	command byte
	address string
	port    uint16
}

func handleRequest(client net.Conn) (*socks5Request, error) {
	// Read request header
	header := make([]byte, 4)
	if _, err := io.ReadFull(client, header); err != nil {
		return nil, fmt.Errorf("failed to read request header: %v", err)
	}

	version := header[0]
	if version != socksVersion5 {
		return nil, fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	command := header[1]
	if command != cmdConnect && command != cmdUDPAssociate { // CONNECT (0x01) or UDP ASSOCIATE (0x03)
		return nil, fmt.Errorf("unsupported command: %d", command)
	}

	// Reserved field should be 0
	if header[2] != reservedField {
		return nil, fmt.Errorf("invalid reserved field: %d", header[2])
	}

	addressType := header[3]
	var address string
	var port uint16

	switch addressType {
	case addrTypeIPv4: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(client, addr); err != nil {
			return nil, fmt.Errorf("failed to read IPv4 address: %v", err)
		}
		address = net.IP(addr).String()

	case addrTypeDomain: // Domain name
		domainLen := make([]byte, 1)
		if _, err := io.ReadFull(client, domainLen); err != nil {
			return nil, fmt.Errorf("failed to read domain length: %v", err)
		}
		domain := make([]byte, domainLen[0])
		if _, err := io.ReadFull(client, domain); err != nil {
			return nil, fmt.Errorf("failed to read domain: %v", err)
		}
		address = string(domain)

	case addrTypeIPv6: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(client, addr); err != nil {
			return nil, fmt.Errorf("failed to read IPv6 address: %v", err)
		}
		address = net.IP(addr).String()

	default:
		return nil, fmt.Errorf("unsupported address type: %d", addressType)
	}

	// Read port
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(client, portBytes); err != nil {
		return nil, fmt.Errorf("failed to read port: %v", err)
	}
	port = binary.BigEndian.Uint16(portBytes)

	slog.Info(fmt.Sprintf("SOCKS5 request: %s:%d", address, port))

	return &socks5Request{
		command: command,
		address: address,
		port:    port,
	}, nil
}

func handleRelay(client net.Conn, request *socks5Request) error {
	switch request.command {
	case cmdConnect: // CONNECT command
		return handleTCPRelay(client, request)
	case cmdUDPAssociate: // UDP ASSOCIATE command
		return handleUDPRelay(client, request)
	default:
		return fmt.Errorf("unsupported command: %d", request.command)
	}
}

type targetWs struct {
	conn        *websocket.Conn
	isFirstRead bool
}

func makeTargetWs(addr, uid string, req *socks5Request) (*targetWs, error) {
	target, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target %s: %v", addr, err)
	}
	udpOrTcp := "tcp"
	if req.command == cmdUDPAssociate {
		udpOrTcp = "udp"
	}
	vlessHeadData := MakeVless(uid, req.address, req.port, udpOrTcp, nil).DataHeader()
	err = target.WriteMessage(websocket.BinaryMessage, vlessHeadData)
	if err != nil {
		return nil, fmt.Errorf("failed to send VLESS header: %w", err)
	}
	return &targetWs{conn: target, isFirstRead: true}, nil
}

func (t *targetWs) Read(p []byte) (n int, err error) {
	_, bytesRead, err := t.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	if len(bytesRead) > 2 && t.isFirstRead {
		// Handle the first read case
		t.isFirstRead = false
		bytesRead = bytesRead[2:] // Skip the first two bytes
	}
	return copy(p, bytesRead), nil
}

func (t *targetWs) NextRead(p []byte) (n int, err error) {
	t.conn.NextReader()
	_, bytesRead, err := t.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	if len(bytesRead) > 2 && t.isFirstRead {
		// Handle the first read case
		t.isFirstRead = false
		bytesRead = bytesRead[2:] // Skip the first two bytes
	}
	return copy(p, bytesRead), nil
}

func (t *targetWs) Write(p []byte) (n int, err error) {
	err = t.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *targetWs) Close() error {
	return t.conn.Close()
}

func handleTCPRelay(client net.Conn, request *socks5Request) error {
	// Connect to target
	target, err := makeTargetWs(wsURL, vlessUUID, request)
	if err != nil {
		// Send failure response
		response := []byte{socksVersion5, replyHostUnreachable, reservedField, addrTypeIPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		client.Write(response)
		return fmt.Errorf("failed to connect to target %s:%d %v", request.address, request.port, err)
	}
	defer target.Close()

	// Send success response
	// For CONNECT requests, we should return the address and port that was used
	// Since we're connecting to the target, we'll return the target's address
	response := make([]byte, responseFixedLen)
	response[0] = socksVersion5  // Version
	response[1] = replySucceeded // Success
	response[2] = reservedField  // Reserved
	response[3] = addrTypeIPv4   // IPv4 address type

	// The request.address contains the host, and request.port contains the port
	// For domain names, we'll use 0.0.0.0
	copy(response[4:8], net.IPv4zero)
	slog.Debug(fmt.Sprintf("Using fallback IPv4 address for domain: %s", request.address))
	// Set the port
	binary.BigEndian.PutUint16(response[8:10], request.port)
	slog.Debug(fmt.Sprintf("Response port: %d", request.port))

	if _, err := client.Write(response); err != nil {
		return fmt.Errorf("failed to write success response: %v", err)
	}

	slog.Info(fmt.Sprintf("TCP relay established between %s and %s", client.RemoteAddr(), request.address))

	// Start bidirectional relay
	errChan := make(chan error, 2)

	// Client -> Target
	go func() {
		_, err := io.Copy(target, client)
		errChan <- err
	}()

	// Target -> Client
	go func() {
		_, err := io.Copy(client, target)
		errChan <- err
	}()

	// Wait for either direction to finish
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil && err != io.EOF {
			slog.Debug(fmt.Sprintf("TCP relay error: %v", err))
		}
	}

	return nil
}

func handleUDPRelay(client net.Conn, request *socks5Request) error {
	// For UDP ASSOCIATE, we need to create a UDP listener
	// The client will send UDP packets to this listener
	udpAddr, err := net.ResolveUDPAddr(networkUDP, localhostAnyPort)
	if err != nil {
		response := []byte{socksVersion5, replyHostUnreachable, reservedField, addrTypeIPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		client.Write(response)
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	udpListener, err := net.ListenUDP(networkUDP, udpAddr)
	if err != nil {
		response := []byte{socksVersion5, replyHostUnreachable, reservedField, addrTypeIPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		client.Write(response)
		return fmt.Errorf("failed to create UDP listener: %v", err)
	}
	defer udpListener.Close()

	// Get the actual address and port that was bound
	actualAddr := udpListener.LocalAddr().(*net.UDPAddr)

	// Send success response with the UDP listener address
	// For UDP ASSOCIATE, we send back the address where the client should send UDP packets
	response := make([]byte, responseFixedLen)
	response[0] = socksVersion5  // Version
	response[1] = replySucceeded // Success
	response[2] = reservedField  // Reserved
	response[3] = addrTypeIPv4   // IPv4 address type

	// Copy the IP address (4 bytes)
	copy(response[4:8], actualAddr.IP.To4())

	// Copy the port (2 bytes, big endian)
	binary.BigEndian.PutUint16(response[8:10], uint16(actualAddr.Port))

	if _, err := client.Write(response); err != nil {
		return fmt.Errorf("failed to write UDP response: %v", err)
	}

	slog.Info(fmt.Sprintf("UDP association established on %s for client %s", actualAddr.String(), client.RemoteAddr()))

	// Monitor client connection and close UDP listener when client disconnects
	go func() {
		defer udpListener.Close()
		buf := make([]byte, 1)
		for {
			_, err := client.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Start UDP relay
	return handleUDPDataRelay(udpListener, client)
}

func handleUDPDataRelay(udpListener *net.UDPConn, client net.Conn) error {
	buffer := make([]byte, maxUDPPacketSize) // Max UDP packet size

	for {
		// Read UDP packet from client
		n, clientAddr, err := udpListener.ReadFromUDP(buffer)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			slog.Debug(fmt.Sprintf("UDP read error: %v", err))
			continue
		}

		// Parse SOCKS5 UDP request header
		if n < udpHeaderLen { // Minimum header size
			continue
		}

		// Debug: log the first few bytes of the UDP packet
		if n > 20 {
			slog.Debug(fmt.Sprintf("UDP packet bytes: %v", buffer[:20]))
		} else {
			slog.Debug(fmt.Sprintf("UDP packet bytes: %v", buffer[:n]))
		}

		// Extract target address from UDP packet
		targetAddr, payload, err := parseUDPRequestHeader(buffer[:n])
		if err != nil {
			slog.Debug(fmt.Sprintf("Failed to parse UDP header: %v", err))
			continue
		}

		slog.Debug(fmt.Sprintf("Parsed UDP target address: %s", targetAddr))

		// Forward the UDP packet to the target
		go func(data []byte, target string) {
			if err := forwardUDPPacket(udpListener, clientAddr, target, data); err != nil {
				slog.Debug(fmt.Sprintf("UDP forward error: %v", err))
			}
		}(payload, targetAddr)
	}
}

func parseUDPRequestHeader(data []byte) (addr string, payload []byte, err error) {
	if len(data) < udpHeaderLen {
		return "", nil, fmt.Errorf("insufficient data for UDP header")
	}

	/*
			Each UDP datagram carries a UDP request
		    header with it:


			      +----+------+------+----------+----------+----------+
			      |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
			      +----+------+------+----------+----------+----------+
			      | 2  |  1   |  1   | Variable |    2     | Variable |
			      +----+------+------+----------+----------+----------+
	*/

	// Skip RSV and FRAG fields
	addrType := data[3]
	var address string
	var port uint16

	switch addrType {
	case addrTypeIPv4: // IPv4
		if len(data) < udpHeaderLen {
			return "", nil, fmt.Errorf("insufficient data for IPv4 address")
		}
		address = net.IP(data[4:8]).String()
		port = binary.BigEndian.Uint16(data[8:10])
		payload = data[10:]
		return net.JoinHostPort(address, strconv.Itoa(int(port))), payload, nil
	case addrTypeDomain: // Domain name
		domainLen := int(data[4])
		if len(data) < 7+domainLen {
			return "", nil, fmt.Errorf("insufficient data for domain name")
		}
		address = string(data[5 : 5+domainLen])
		port = binary.BigEndian.Uint16(data[5+domainLen : 7+domainLen])
		payload = data[7+domainLen:]
		return net.JoinHostPort(address, strconv.Itoa(int(port))), payload, nil

	case addrTypeIPv6: // IPv6
		if len(data) < 22 {
			return "", nil, fmt.Errorf("insufficient data for IPv6 address")
		}
		address = net.IP(data[4:20]).String()
		port = binary.BigEndian.Uint16(data[20:22])
		payload = data[22:]
		return net.JoinHostPort(address, strconv.Itoa(int(port))), payload, nil

	default:
		return "", nil, fmt.Errorf("unsupported address type: %d", addrType)
	}
}

func forwardUDPPacket(udpListener *net.UDPConn, clientAddr *net.UDPAddr, targetAddr string, data []byte) error {
	//has a bug not working
	slog.Debug(fmt.Sprintf("Forwarding UDP packet to target %s", targetAddr))
	// Resolve target address
	addr, err := net.ResolveUDPAddr(networkUDP, targetAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve target address: %v", err)
	}
	req := &socks5Request{command: cmdUDPAssociate, address: addr.AddrPort().Addr().String(), port: addr.AddrPort().Port()}
	ws, err := makeTargetWs(wsURL, vlessUUID, req)
	if err != nil {
		return fmt.Errorf("failed to connect to target %v websocket: %w", addr, err)
	}
	defer ws.Close()

	udpData := vlessUdpDataMake(data)
	if _, err := ws.Write(udpData); err != nil {
		return fmt.Errorf("failed to write to target websocket: %w", err)
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelFunc()

	responseBuffer := make([]byte, maxUDPPacketSize)
	for {
		//todo read websocket has a bug
		select {

		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for response from target websocket")

		default:
			// Read response from ws
			responseBuffer := make([]byte, maxUDPPacketSize)
			n, err := ws.Read(responseBuffer)
			if err != nil {
				if err == io.EOF {
					return fmt.Errorf("connection closed by target websocket")
				}
				slog.Debug(fmt.Sprintf("Read error from target websocket: %v", err))
				continue
			}
			if n > 0 {
				responseBuffer = responseBuffer[:n]
				break
			}
		}
	}
	// Create response packet with SOCKS5 header
	responsePacket := createUDPResponsePacket(data[:udpHeaderLen], responseBuffer)

	// Send response back to client
	_, err = udpListener.WriteToUDP(responsePacket, clientAddr)
	if err != nil {
		return fmt.Errorf("failed to send response to client: %v", err)
	}

	return nil
}

func createUDPResponsePacket(header []byte, data []byte) []byte {
	response := make([]byte, len(header)+len(data))
	copy(response, header)
	copy(response[len(header):], data)
	return response
}
