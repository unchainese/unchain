package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type App struct {
	userUsedTrafficKb sync.Map // string -> int64
	svr               *http.Server
	exitSignal        chan os.Signal
	bufferPool        *sync.Pool
	upGrader          *websocket.Upgrader
}

func (app *App) httpSvr() {
	mux := http.NewServeMux()
	mux.HandleFunc("/wsv/{uid}", app.WsVLESS)
	mux.HandleFunc("/sub/{uid}", app.Sub)
	mux.HandleFunc("/ws-vless", app.WsVLESS)
	mux.HandleFunc("/", app.Ping)

	// pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:         Cfg().ListenAddr(),
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	app.svr = server

}

func NewApp(c *Config, sig chan os.Signal) *App {
	bufferSize := c.GetBufferSize()
	app := &App{
		userUsedTrafficKb: sync.Map{},
		exitSignal:        sig,
		svr:               nil,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, bufferSize)
			},
		},
		upGrader: &websocket.Upgrader{
			HandshakeTimeout: 2 * time.Second,
			ReadBufferSize:   bufferSize,
			WriteBufferSize:  bufferSize,
			CheckOrigin: func(r *http.Request) bool {
				// Allow all connections by default
				return true
			},
		},
	}
	for _, userID := range c.UserIDS() {
		app.userUsedTrafficKb.Store(userID, int64(0))
	}
	app.httpSvr()
	go app.loopPush()
	return app
}

func (app *App) Run() {
	log.Println("server starting on http://", Cfg().ListenAddr())
	if err := app.svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Could not listen on %s: %v\n", Cfg().ListenAddr(), err)
	}
}

func (app *App) PrintVLESSConnectionURLS() {
	subAddr := Cfg().SubAddr()

	fmt.Printf("visit to get VLESS connection info: http://%s/sub/<YOUR_UUID>\n", subAddr)

	app.userUsedTrafficKb.Range(func(id, _ interface{}) bool {
		userID := id.(string)
		fmt.Println("\n------------- USER UUID:  ", userID, " -------------")
		urls := app.vlessUrls(userID)
		for _, url := range urls {
			fmt.Println(url)
		}
		return true
	})
	fmt.Println("\n\n\n")
}

func (app *App) Shutdown(ctx context.Context) {
	log.Println("Shutting down the server...")
	if err := app.svr.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exiting")
}

func (app *App) loopPush() {
	url := Cfg().RegisterUrl
	if url == "" {
		log.Println("Register url is empty, skip register, runs in standalone mode")
		url = "https://unchainapi.bob99.workers.dev/api/node"
	}
	tk := time.NewTicker(Cfg().PushInterval())
	defer tk.Stop()
	for {
		select {
		case sig := <-app.exitSignal:
			app.exitSignal <- sig
			app.PushNode() //last push
			return
		case <-tk.C:
			app.PushNode()
		}
	}
}

func (app *App) trafficInc(uid string, byteN int64) {
	if !Cfg().EnableMetering {
		return
	}
	kb := byteN >> 10
	value, ok := app.userUsedTrafficKb.Load(uid)
	if ok {
		iv, isInt64 := value.(int64)
		if isInt64 {
			kb += iv
		} else {
			slog.Error("not a int64", "uid", uid, "value", value)
		}
	}
	app.userUsedTrafficKb.Store(uid, kb)
}

func (app *App) stat() *AppStat {
	data := make(map[string]int64)
	app.userUsedTrafficKb.Range(func(key, value interface{}) bool {
		data[key.(string)] = value.(int64)
		return true
	})
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
		slog.Error(err.Error())
	}

	res := &AppStat{
		Traffic:     data,
		Hostname:    hostname,
		Goroutine:   int64(runtime.NumGoroutine()),
		VersionInfo: Cfg().GitHash + " -> " + Cfg().BuildTime,
	}
	return res
}

type AppStat struct {
	Traffic     map[string]int64 `json:"traffic"`
	Hostname    string           `json:"hostname"`
	Ip          string           `json:"ip"`
	Goroutine   int64            `json:"goroutine"`
	VersionInfo string           `json:"version_info"`
}

func (app *App) PushNode() {
	url := Cfg().RegisterUrl
	if url == "" {
		return
	}
	args := app.stat()
	body := bytes.NewBuffer(nil)
	err := json.NewEncoder(body).Encode(args)
	if err != nil {
		log.Println("Error encoding request:", err)
		return
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		log.Println("Error registering:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", Cfg().RegisterToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error registering:", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		all, err := io.ReadAll(resp.Body) //read and discard body
		if err != nil {
			log.Println("Error reading response body:", err)
		}
		log.Println("Error registering, status code:", resp.StatusCode, "body:", string(all))
		return
	}
	defer resp.Body.Close()
	users := make(map[string]int64)
	err = json.NewDecoder(resp.Body).Decode(&users)
	if err != nil {
		log.Println("Error decoding response:", err)
		return
	}
	app.userUsedTrafficKb.Clear()
	const dummyUsedKB = int64(0)
	for k, userAvailableKB := range users {
		slog.Debug("user available traffic", "uid", k, "available", userAvailableKB)
		app.userUsedTrafficKb.Store(k, dummyUsedKB) //set allowed userID
	}
	Cfg().UserIDS()
	//append config file UUIDs
	for _, id := range Cfg().UserIDS() {
		app.userUsedTrafficKb.Store(id, dummyUsedKB)
	}
}

func (app *App) IsUserNotAllowed(uuid string) (isNotAllowed bool) {
	_, ok := app.userUsedTrafficKb.Load(uuid)
	return !ok
}
