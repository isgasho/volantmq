// Copyright (c) 2014 The SurgeMQ Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/juju/loggo"
	"github.com/troian/surgemq"
	"github.com/troian/surgemq/auth"
	"github.com/troian/surgemq/server"
	"syscall"
)

var (
	keepAlive        int
	connectTimeout   int
	ackTimeout       int
	timeoutRetries   int
	authenticator    string
	sessionsProvider string
	topicsProvider   string
	cpuprofile       string
	wsAddr           string // HTTPS websocket address eg. :8080
	wssAddr          string // HTTPS websocket address, eg. :8081
	wssCertPath      string // path to HTTPS public key
	wssKeyPath       string // path to HTTPS private key
)

func init() {
	flag.IntVar(&keepAlive, "keepalive", surgemq.DefaultAckTimeout, "Keepalive (sec)")
	flag.IntVar(&connectTimeout, "connecttimeout", surgemq.DefaultConnectTimeout, "Connect Timeout (sec)")
	flag.IntVar(&ackTimeout, "acktimeout", surgemq.DefaultAckTimeout, "Ack Timeout (sec)")
	flag.IntVar(&timeoutRetries, "retries", surgemq.DefaultTimeoutRetries, "Timeout Retries")
	flag.StringVar(&authenticator, "auth", surgemq.DefaultAuthenticator, "Authenticator Type")
	flag.StringVar(&sessionsProvider, "sessions", surgemq.DefaultSessionsProvider, "Session Provider Type")
	flag.StringVar(&topicsProvider, "topics", surgemq.DefaultTopicsProvider, "Topics Provider Type")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "CPU Profile Filename")
	flag.StringVar(&wsAddr, "wsaddr", "", "HTTP websocket address, eg. ':8080'")
	flag.StringVar(&wssAddr, "wssaddr", "", "HTTPS websocket address, eg. ':8081'")
	flag.StringVar(&wssCertPath, "wsscertpath", "", "HTTPS server public key file")
	flag.StringVar(&wssKeyPath, "wsskeypath", "", "HTTPS server private key file")
	flag.Parse()
}

var appLog loggo.Logger

func main() {
	var f *os.File

	svr, _ := server.New(server.Config{
		KeepAlive:      keepAlive,
		ConnectTimeout: connectTimeout,
		AckTimeout:     ackTimeout,
		TimeoutRetries: timeoutRetries,
		TopicsProvider: topicsProvider,
	})

	var err error

	if cpuprofile != "" {
		f, err = os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}

		pprof.StartCPUProfile(f) // nolint: errcheck
	}

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigchan
		appLog.Errorf("Existing due to trapped signal; %v", sig)

		if f != nil {
			appLog.Errorf("Stopping profile")
			pprof.StopCPUProfile()
			f.Close() // nolint: errcheck
		}

		svr.Close() // nolint: errcheck

		os.Exit(0)
	}()

	if len(wsAddr) > 0 || len(wssAddr) > 0 {
		addr := "tcp://127.0.0.1:1883"
		addWebSocketHandler("/mqtt", addr) // nolint: errcheck
		/* start a plain websocket listener */
		if len(wsAddr) > 0 {
			go listenAndServeWebSocket(wsAddr) // nolint: errcheck
		}
		/* start a secure websocket listener */
		if len(wssAddr) > 0 && len(wssCertPath) > 0 && len(wssKeyPath) > 0 {
			go listenAndServeWebSocketSecure(wssAddr, wssCertPath, wssKeyPath) // nolint: errcheck
		}
	}

	authMng, err := auth.NewManager("mockSuccess")
	if err != nil {
		appLog.Errorf("Couldn't register *mockSuccess* auth provider: %s", err.Error())
		return
	}

	config := &server.Listener{
		Scheme:      "tcp",
		Host:        "127.0.0.1",
		Port:        1883,
		AuthManager: authMng,
	}
	/* create plain MQTT listener */
	err = svr.ListenAndServe(config)
	if err != nil {
		appLog.Errorf("surgemq/main: %v", err)
	}
}
