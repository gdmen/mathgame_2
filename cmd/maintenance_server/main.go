// maintenance_server serves a static "be right back" page for every path
// while the real web server and apiserver are stopped during a deploy
// (deploy/update.sh swaps it in for mathgame-web around the disruptive
// window). With tls_cert_file/tls_key_file set in the config it serves
// HTTPS on the web server's port; with either empty it serves plain HTTP
// (local testing).
package main

import (
	_ "embed"
	"flag"
	"net/http"

	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

//go:embed maintenance.html
var page []byte

// Handler answers every path with the maintenance page and a 503 so
// clients and crawlers treat the outage as temporary.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Retry-After", "120")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write(page)
}

func main() {
	configPath := flag.String("config", "conf.json", "path to config file")
	port := flag.String("port", "443", "port to listen on")
	flag.Parse()

	c, err := common.ReadConfig(*configPath)
	if err != nil {
		glog.Fatalf("read config: %v", err)
	}

	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		glog.Fatal("config sets only one of tls_cert_file/tls_key_file; need both (or neither for plain HTTP)")
	}

	addr := ":" + *port
	http.HandleFunc("/", Handler)
	if c.TLSCertFile != "" && c.TLSKeyFile != "" {
		glog.Infof("serving maintenance page on https %s", addr)
		glog.Fatal(http.ListenAndServeTLS(addr, c.TLSCertFile, c.TLSKeyFile, nil))
	}
	glog.Infof("serving maintenance page on http %s (no TLS configured)", addr)
	glog.Fatal(http.ListenAndServe(addr, nil))
}
