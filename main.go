package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fortio.org/fortio/log"
	"fortio.org/fortio/stats"
	"github.com/miekg/dns"
)

func usage() {
	fmt.Println("Usage: dnsping query server\neg:\tdnsping www.google.com. 127.0.0.1")
	os.Exit(1)
}

func main() {
	portFlag := flag.Int("p", 53, "`Port` to connect to")
	intervalFlag := flag.Duration("i", 1*time.Second, "How long to `wait` between requests")
	countFlag := flag.Int("c", 0, "How many `requests` to make. Default is to run until ^C")
	queryTypeFlag := flag.String("t", "A", "Query `type` to use (A, SOA, CNAME...)")
	flag.Parse()
	qt, exists := dns.StringToType[*queryTypeFlag]
	if !exists {
		log.Errf("Invalid type name %q", *queryTypeFlag)
		os.Exit(1)
	}
	args := flag.Args()
	nArgs := len(args)
	log.LogVf("got %d arguments: %v", nArgs, args)
	if nArgs != 2 {
		usage()
	}
	addrStr := fmt.Sprintf("%s:%d", args[1], *portFlag)
	DNSPing(addrStr, args[0], qt, *countFlag, *intervalFlag)
}

// DNSPing Runs the query howMany times against addrStr server, sleeping for interval time.
func DNSPing(addrStr, queryStr string, queryType uint16, howMany int, interval time.Duration) {
	m := new(dns.Msg)
	m.SetQuestion(queryStr, queryType)
	qtS := dns.TypeToString[queryType]
	log.Infof("Will query server: %s for %s (%d) record for %s", addrStr, qtS, queryType, queryStr)
	log.LogVf("Query is: %v", m)
	successCount := 0
	errorCount := 0
	stats := stats.NewHistogram(0, 0.1)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	continueRunning := true
	for i := 1; continueRunning && (howMany <= 0 || i <= howMany); i++ {
		if i != 1 {
			select {
			case <-ch:
				continueRunning = false
				fmt.Println()
				continue
			case <-time.After(interval):
			}
		}
		start := time.Now()
		r, err := dns.Exchange(m, addrStr)
		durationMS := 1000. * time.Since(start).Seconds()
		stats.Record(durationMS)
		if err != nil {
			log.Errf("%6.1f ms %3d: failed call: %v", durationMS, i, err)
			errorCount++
			continue
		}
		if r == nil {
			log.Critf("bug? dns response is nil")
			errorCount++
			continue
		}
		log.LogVf("response is %v", r)
		if r.Rcode != dns.RcodeSuccess {
			log.Errf("%6.1f ms %3d: server error: %v", durationMS, i, err)
			errorCount++
		} else {
			successCount++
		}
		log.Infof("%6.1f ms %3d: %v", durationMS, i, r.Answer)
	}
	perc := fmt.Sprintf("%.02f%%", 100.*float64(errorCount)/float64(errorCount+successCount))
	fmt.Printf("%d errors (%s), %d success.\n", errorCount, perc, successCount)
	res := stats.Export()
	res.CalcPercentiles([]float64{50, 90, 99})
	res.Print(os.Stdout, "response time (in ms)")
}
