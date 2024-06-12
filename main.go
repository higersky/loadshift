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
	"sync"
	"time"
)

type HostInfo struct {
	URL     *url.URL
	Latency time.Duration
}

const ERROR_TORERLATE_MAX int = 2
const ERROR_RECHECK_MAX int = 2

var stateMutex sync.RWMutex
var errorStateMutex sync.Mutex

var errorTolerateCount = ERROR_TORERLATE_MAX
var errorRecheckCount = ERROR_RECHECK_MAX

func main() {
	// Parse command line flags
	listenPort := flag.Int("port", 8080, "Port to listen on")
	hostAddresses := flag.String("hosts", "host1.example.com:80,host2.example.com:80", "Comma-separated list of host addresses")
	fallback := flag.String("fallback", "localhost:80", "Fallback host when all hosts failed")
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
	u, err := url.Parse("http://" + *fallback)
	if err != nil {
		log.Fatalf("Invalid fallback address: %s", *fallback)
	}
	fallbackHost := &HostInfo{URL: u}
	serviceAvailable := true

	// Start periodic latency checks
	go checkHostLatencyLoop(hosts, *checkInterval, &serviceAvailable)

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", *listenPort),
	})
	proxy.Director = func(req *http.Request) {
		// Select the fastest host
		fastestHost := getFastestHost(hosts, fallbackHost, &serviceAvailable)
		req.URL.Scheme = fastestHost.URL.Scheme
		req.URL.Host = fastestHost.URL.Host
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("http: proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		errorStateMutex.Lock()
		errorTolerateCount--
		if errorTolerateCount == 0 && errorRecheckCount > 0 {
			log.Printf("Re-checking host latency")
			go checkHostLatency(hosts, &serviceAvailable)
			errorRecheckCount--
			errorTolerateCount = ERROR_TORERLATE_MAX
		}
		errorStateMutex.Unlock()
	}

	// Start the server
	log.Printf("Starting reverse proxy on localhost:%d", *listenPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", *listenPort), proxy))
}

func checkHostLatencyLoop(hosts []*HostInfo, interval time.Duration, serviceAvailable *bool) {
	for {
		checkHostLatency(hosts, serviceAvailable)
		time.Sleep(interval)
		errorStateMutex.Lock()
		errorTolerateCount = ERROR_TORERLATE_MAX
		errorRecheckCount = ERROR_RECHECK_MAX
		errorStateMutex.Unlock()
	}
}

func checkHostLatency(hosts []*HostInfo, serviceAvailable *bool) {
	newStatus := false
	tmp := make([]time.Duration, len(hosts))

	stateMutex.RLock()
	for i, host := range hosts {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", host.URL.Host, 2*time.Second)
		if err != nil {
			tmp[i] = time.Duration(^uint64(0) >> 1)
			log.Printf("Error checking latency for %s: %v", host.URL, err)
			continue
		}
		newStatus = true
		tmp[i] = time.Since(start)
		conn.Close()
		log.Printf("Latency for %s: %v", host.URL, tmp[i])
	}
	stateMutex.RUnlock()

	stateMutex.Lock()
	*serviceAvailable = newStatus
	for i, host := range hosts {
		host.Latency = tmp[i]
	}
	if newStatus {
		slices.SortFunc(hosts, func(i, j *HostInfo) int {
			return cmp.Compare(i.Latency, j.Latency)
		})
	}
	stateMutex.Unlock()

	if newStatus {
		stateMutex.RLock()
		log.Printf("The fastest is %s", hosts[0].URL)
		stateMutex.RUnlock()
	} else {
		log.Printf("Fallback enabled")
	}
}

func getFastestHost(hosts []*HostInfo, fallbackHost *HostInfo, serviceAvailable *bool) *HostInfo {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	if !*serviceAvailable {
		return fallbackHost
	}
	return hosts[0]
}
