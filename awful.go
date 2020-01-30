package main

import (
	"flag"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var ip = flag.String("ip", "127.0.0.1", "ip address of this server")
var listen = flag.String("listen", ":1053", "port to listen on")
var basename = flag.String("base", "example.com", "domain on which this is configured in the public DNS.")

func main() {
	flag.Parse()

	mux := dns.NewServeMux()

	udpServer := &dns.Server{
		Addr:    *listen,
		Net:     "udp",
		Handler: mux,
	}
	tcpServer := &dns.Server{
		Addr:    *listen,
		Net:     "tcp",
		Handler: mux,
	}

	mux.HandleFunc("cnamepit."+*basename, cnamePitHandler)
	mux.HandleFunc("manycuts."+*basename, manyCutsHandler)
	mux.HandleFunc("sleep."+*basename, sleepHandler)
	mux.HandleFunc(".", unknownHandler)

	errChan := make(chan error)
	go func() {
		errChan <- udpServer.ListenAndServe()
	}()
	go func() {
		errChan <- tcpServer.ListenAndServe()
	}()

	err := <-errChan
	if err != nil {
		log.Fatal(err)
	}
}

// qname returns the QNAME from a query. If there is no QNAME in a query it
// returns ".".
func qname(q *dns.Msg) string {
	if len(q.Question) > 0 {
		return q.Question[0].Name
	}
	return "."
}

func logQuery(w dns.ResponseWriter, q *dns.Msg, handler string) {
	log.Printf("query from %s for %q, handled by %s",
		w.RemoteAddr(), qname(q), handler)
}

// unknownHandler handles any request that doesn't match a pattern.
func unknownHandler(w dns.ResponseWriter, q *dns.Msg) {
	logQuery(w, q, "unknownHandler")
	txtError(w, q, "request did not match any known pattern.")
}

// txtError writes a response with a TXT record containing the given error
// message.
func txtError(w dns.ResponseWriter, q *dns.Msg, errorMsg string) {
	m := new(dns.Msg)
	m.SetRcode(q, dns.RcodeSuccess)

	m.Answer = []dns.RR{
		&dns.TXT{
			Hdr: dns.RR_Header{
				Name:   qname(q),
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
			},
			Txt: []string{errorMsg},
		},
	}
	w.WriteMsg(m)
}

// cnamePitHandler answers every query with a CNAME to a name formed by
// prepending "q." to its own name, causing recursors to chase the CNAMEs
// until they give up.
func cnamePitHandler(w dns.ResponseWriter, q *dns.Msg) {
	logQuery(w, q, "cnamePitHandler")
	m := new(dns.Msg)
	m.SetRcode(q, dns.RcodeSuccess)
	record := &dns.CNAME{
		Hdr: dns.RR_Header{
			Name:   qname(q),
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
		},
		Target: "q." + qname(q),
	}
	m.Answer = []dns.RR{record}
	w.WriteMsg(m)
}

// manyCutsHandler always replies with a referral.
func manyCutsHandler(w dns.ResponseWriter, q *dns.Msg) {
	logQuery(w, q, "manyCutsHandler")
	m := new(dns.Msg)
	m.SetRcode(q, dns.RcodeSuccess)
	name := q.Question[0].Name
	nextName := "q." + name
	record := &dns.NS{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeNS,
			Class:  dns.ClassINET,
		},
		Ns: nextName,
	}
	m.Ns = []dns.RR{record}
	m.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   nextName,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.ParseIP(*ip),
		},
	}

	w.WriteMsg(m)
}

// sleepHandler sleeps the number of milliseconds specified in the first label of the
// qname, and then replies with NOERROR. If the label fails to parse it will
// return a TXT record with an error message.
func sleepHandler(w dns.ResponseWriter, q *dns.Msg) {
	logQuery(w, q, "sleepHandler")
	m := new(dns.Msg)
	m.SetRcode(q, dns.RcodeSuccess)

	labels := strings.Split(qname(q), ".")
	sleepCount, err := strconv.ParseInt(labels[0], 10, 16)
	if err != nil {
		txtError(w, q, "failed to parse integer sleep time")
		return
	}

	time.Sleep(time.Duration(sleepCount) * time.Millisecond)
	w.WriteMsg(m)
}
