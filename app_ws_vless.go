package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
	upgradeHeader     = "Upgrade"
	websocketProtocol = "websocket"
	secWebSocketProto = "sec-websocket-protocol"
)

func startDstConnection(vd *ProtoVLESS, timeout time.Duration) (net.Conn, []byte, error) {
	conn, err := net.DialTimeout(vd.DstProtocol, vd.HostPort(), timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to destination: %w", err)
	}
	return conn, []byte{vd.Version, 0x00}, nil
}

func (app *App) WsVLESS(w http.ResponseWriter, r *http.Request) {

	uid := r.PathValue("uid")
	//check can upgrade websocket
	if r.Header.Get(upgradeHeader) != websocketProtocol {
		//json response hello world
		w.Header().Set(contentTypeHeader, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		data := map[string]string{"msg": "pong", "uid": uid}
		json.NewEncoder(w).Encode(data)
		return
	}

	ctx := r.Context()
	earlyDataHeader := r.Header.Get(secWebSocketProto)
	earlyData, err := base64.RawURLEncoding.DecodeString(earlyDataHeader)
	if err != nil {
		log.Println("Error decoding early data:", err)
	}

	ws, err := app.upGrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading to websocket:", err)
		return
	}
	defer ws.Close()

	if len(earlyData) == 0 {
		mt, p, err := ws.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			return
		}
		if mt == websocket.BinaryMessage {
			earlyData = p
		}
	}

	vData, err := VLESSParse(earlyData)
	if err != nil {
		log.Println("Error parsing vless data:", err)
		return
	}
	if app.IsUserNotAllowed(vData.UUID()) {
		return
	}

	sessionTrafficByteN := int64(len(earlyData))

	if vData.DstProtocol == "udp" {
		sessionTrafficByteN += app.vlessUDP(ctx, vData, ws)
	} else if vData.DstProtocol == "tcp" {
		sessionTrafficByteN += app.vlessTCP(ctx, vData, ws)
	} else {
		log.Println("Error unsupported protocol:", vData.DstProtocol)
		return
	}
	go app.trafficInc(vData.UUID(), sessionTrafficByteN)
}

const readTimeOut = 60 * time.Second * 3

func (app *App) vlessTCP(ctx context.Context, sv *ProtoVLESS, ws *websocket.Conn) int64 {
	logger := sv.Logger()
	conn, headerVLESS, err := startDstConnection(sv, time.Millisecond*1000)
	if err != nil {
		logger.Error("Error starting session:", "err", err)
		return 0
	}
	defer conn.Close()
	logger.Info("Session started tcp")

	//write early data
	_, err = conn.Write(sv.DataTcp())
	if err != nil {
		logger.Error("Error writing early data to TCP connection:", "err", err)
		return 0
	}
	var trafficMeter atomic.Int64
	var wg sync.WaitGroup

	// Create cancellable context for proper goroutine cleanup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Ensure both goroutines exit when function returns

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel() // Cancel context if this goroutine exits
		for {
			select {
			case <-ctx.Done():
				return
			default:
				ws.SetReadDeadline(time.Now().Add(readTimeOut))
				mt, message, err := ws.ReadMessage()
				trafficMeter.Add(int64(len(message)))
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				//handle EOF error
				if errors.Is(err, websocket.ErrCloseSent) {
					return
				}
				if err != nil {
					logger.Error("Error reading message:", "err", err)
					return
				}
				if mt != websocket.BinaryMessage {
					continue
				}
				_, err = conn.Write(message)
				if err != nil {
					logger.Error("Error writing to TCP connection:", "err", err)
					return
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer cancel() // Cancel context if this goroutine exits
		hasNotSentHeader := true
		buf := app.bufferPool.Get().([]byte)
		defer app.bufferPool.Put(buf)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn.SetReadDeadline(time.Now().Add(readTimeOut))
				n, err := conn.Read(buf)
				trafficMeter.Add(int64(n))
				if errors.Is(err, io.EOF) {
					return
				}
				if err != nil {
					logger.Error("Error reading from TCP connection:", "err", err)
					return
				}
				data := buf[:n]
				// send header data only for the first time
				if hasNotSentHeader {
					hasNotSentHeader = false
					data = append(headerVLESS, data...)
				}
				err = ws.WriteMessage(websocket.BinaryMessage, data)
				if err != nil {
					logger.Error("Error writing to websocket:", "err", err)
					return
				}
			}
		}
	}()
	wg.Wait()
	return trafficMeter.Load()
}

// vlessUDP handles UDP traffic over VLESS protocol via WebSocket is tested ok
func (app *App) vlessUDP(_ context.Context, sv *ProtoVLESS, ws *websocket.Conn) (trafficMeter int64) {
	logger := sv.Logger()
	conn, headerVLESS, err := startDstConnection(sv, time.Millisecond*1000)
	if err != nil {
		logger.Error("Error starting session:", "err", err)
		return
	}
	defer conn.Close()
	udpData := sv.DataUdp()
	_, err = conn.Write(udpData)
	if err != nil {
		logger.Error("Error writing early data to UDP connection:", "err", err)
		return
	}

	buf := app.bufferPool.Get().([]byte)
	defer app.bufferPool.Put(buf)
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		logger.Error("Error reading from UDP connection:", "err", err)
		return
	}
	udpDataLen1 := (n >> 8) & 0xff
	udpDataLen2 := n & 0xff
	headerVLESS = append(headerVLESS, byte(udpDataLen1), byte(udpDataLen2))
	headerVLESS = append(headerVLESS, buf[:n]...)

	//send back the first udp packet with vless header
	err = ws.WriteMessage(websocket.BinaryMessage, headerVLESS)
	if err != nil {
		logger.Error("Error writing to websocket:", "err", err)
		return
	}
	return int64(len(headerVLESS)) + int64(len(udpData))
}

func vlessUdpDataMake(payload []byte) []byte {
	n := len(payload)
	allBytes := make([]byte, n+2)
	allBytes[0] = byte((n >> 8) & 0xff)
	allBytes[1] = byte(n & 0xff)
	copy(allBytes[2:], payload)
	return allBytes
}

func vlessUdpDataExtract(data []byte) []byte {
	if len(data) < 2 {
		return nil
	}
	udpDataLen1 := int(data[0])
	udpDataLen2 := int(data[1])
	udpDataLen := (udpDataLen1 << 8) | udpDataLen2
	if len(data) < udpDataLen+2 {
		return nil
	}
	return data[2 : 2+udpDataLen]
}
