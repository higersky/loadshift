package main

import (
	"cmp"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"time"
)

type HostInfo struct {
	URL     *url.URL
	Latency time.Duration
}

func main() {
	// Parse command line flags
	listenPort := flag.Int("port", 8080, "Port to listen on")
	hostAddresses := flag.String("hosts", "host1.example.com:80,host2.example.com:80", "Comma-separated list of host addresses")
	checkInterval := flag.Duration("check-interval", 10*time.Second, "Interval to check host latency")
	flag.Parse()

	// Parse host addresses
	hosts := make([]*HostInfo, 0)
	for _, addr := range strings.Split(*hostAddresses, ",") {
		u, err := url.Parse("http://" + addr)
		if err != nil {
			log.Fatalf("Invalid host address: %s", addr)
		}
		hosts = append(hosts, &HostInfo{URL: u})
	}

	// Start periodic latency checks
	go checkHostLatency(hosts, *checkInterval)

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", *listenPort),
	})
	proxy.Director = func(req *http.Request) {
		// Select the fastest host
		fastestHost := getFastestHost(hosts)
		req.URL.Scheme = fastestHost.URL.Scheme
		req.URL.Host = fastestHost.URL.Host
	}

	// Start the server
	log.Printf("Starting reverse proxy on localhost:%d", *listenPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", *listenPort), proxy))
}

func checkHostLatency(hosts []*HostInfo, interval time.Duration) {
	for {
		for _, host := range hosts {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", host.URL.Host, 5*time.Second)
			if err != nil {
				host.Latency = time.Duration(^uint64(0) >> 1)
				log.Printf("Error checking latency for %s: %v", host.URL, err)
				continue
			}
			host.Latency = time.Since(start)
			conn.Close()
			log.Printf("Latency for %s: %v", host.URL, host.Latency)
		}
		slices.SortFunc(hosts, func(i, j *HostInfo) int {
			return cmp.Compare(i.Latency, j.Latency)
		})
		log.Printf("The fastest is %s", hosts[0].URL)
		time.Sleep(interval)
	}
}

func getFastestHost(hosts []*HostInfo) *HostInfo {
	return hosts[0]
}
