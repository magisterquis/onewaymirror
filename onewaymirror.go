// Onewaymirror accepts a connection, connects back to the port on the peer on
// which onewaymirror is listening, and reflects anything sent to it right
// back, logging the whole session in the process.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

/*
 * onewaymirror.go
 * by J. Stuart McMurray
 * created 20140428
 * last modified 20140428
 */

func main() {
	/* Parse options */
	listenAddr := flag.String("addr", ":23", "[Address and] port on which to listen.")
	//logDir := flag.String("logdir", "onewaymirror", "Directory to which to write logs.")
	//enableLogging := flag.Bool("nolog", false, "Disable session logging.")
	disable4 := flag.Bool("no4", false, "Disable IPv4.")
	disable6 := flag.Bool("no6", false, "Disable IPv6.")
	banner := flag.String("banner", "Connection proxied by onewaymirror.",
		"A banner to send to connecting clients.  This may be set "+
			"to the empty string (-banner=\"\") for no "+
			"banner.  A newline will be appended to the banner "+
			"after sending.")
	buflen := flag.Int("buflen", 1024, "Read buffer size.")
	flag.Parse()

	/* Make sure logdir exists */
	/* TODO: unhardcode the perms */
	//if err := os.MkdirAll(*logDir, 0755); err != nil {
	//	log.Fatalf("Unable to create directory %v: %v", *logDir, err)
	//}

	/* Channels on which to receive connections */
	var ch4, ch6 chan *net.TCPConn

	/* Channels to use for watching for dead listeners */
	dead := make(chan int)
	allDead := make(chan int)
	n := 0

	/* Append a newline to the banner if appropriate */
	if len(*banner) > 0 {
		*banner += "\n"
	}

	/* Try to listen */
	if *disable4 && *disable6 {
		log.Fatalf("-no4 and -no6 may not both be specified.")
	}
	var l4, l6 *net.TCPListener
	if !*disable4 {
		l4 = listener("tcp4", *listenAddr)
		if l4 != nil {
			ch4 = make(chan *net.TCPConn)
			n++
			go waitConn(l4, ch4, dead)
		}
	}
	if !*disable6 {
		l6 = listener("tcp6", *listenAddr)
		if l6 != nil {
			ch6 = make(chan *net.TCPConn)
			n++
			go waitConn(l6, ch6, dead)
		}
	}
	if nil == l4 && nil == l6 {
		log.Fatalf("Unaable to create any listeners")
	}

	/* Start listeners */
	go waitDead(n, dead, allDead)

	/* Accept connections */
	for {
		select {
		case c := <-ch4:
			go handleConn(c, *buflen, *banner)
		case c := <-ch6:
			go handleConn(c, *buflen, *banner)
		case <-allDead:
			log.Fatalf("All listeners have terminated")
		}

	}
	/* Shouldn't reach here */
	log.Fatalf("Unexpected termination")
}

/* Goroutine to handle incoming connection */
/* handleConn handles incoming connections using a buffer of buflen bytes and
sending banner to each incoming connection if banner is not the empty string */
func handleConn(r *net.TCPConn, buflen int, banner string) {
	defer r.Close()
	constr := fmt.Sprintf("%v -> %v", r.RemoteAddr(), r.LocalAddr())
	log.Printf("Connection got: %v", constr)

	/* Send banner to each connecting connection */
	if l := len(banner); l > 0 {
		s := 0
		for s < l {
			n, err := r.Write([]byte(banner))
			if err != nil {
				log.Printf("Unable to send banner to %v: %v",
					r.RemoteAddr(), err)
				return
			}
			s += n
		}
	}

	/* Target */
	ta := &net.TCPAddr{}

	/* Build the target address:port */
	rad, ok := r.RemoteAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("%v is not a TCP address.  Please file a bug "+
			"report.", r.RemoteAddr())
		return
	}
	ta.IP = rad.IP
	lad, ok := r.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("%v is not a TCP address.  Please file a bug "+
			"report.", r.LocalAddr())
		return
	}
	ta.Port = lad.Port

	/* Try to connect right back */
	t, err := net.DialTCP("tcp", nil, ta)
	if err != nil {
		e := fmt.Sprintf("Unable to connect back to %v\n", ta)
		log.Printf("%v: %v", e, err)
		if _, err := r.Write([]byte(e)); err != nil {
			log.Printf("Unable to tell %v a connection can't be "+
				"established to %v: %v", rad, ta, err)
		}
		return
	}
	defer t.Close()
	tgtstr := fmt.Sprintf("%v -> %v", t.LocalAddr(), t.RemoteAddr())
	log.Printf("Connection made: %v", tgtstr)

	/* Proxy bytes */
	done := make(chan *net.TCPConn)
	go proxyBytes(r, t, done, buflen, constr)
	go proxyBytes(t, r, done, buflen, tgtstr)

	/* Close both sides when one closes */
	<-done
}

/* Make a TCPListener for the specified tcp family: tcp4 or tcp6 */
func listener(t, addr string) *net.TCPListener {
	if t != "tcp4" && t != "tcp6" {
		panic("listener() takes either \"tcp4\" or \"tcp6\" as its " +
			"first argument.")
	}
	tcpAddr, err := net.ResolveTCPAddr(t, addr)
	if err != nil {
		log.Fatalf("Unable to resolve %v address %v: %v", t, addr, err)
		return nil
	}
	l, err := net.ListenTCP(t, tcpAddr)
	if err != nil {
		log.Fatalf("Unable to listen on %v: %v", tcpAddr, err)
		return nil
	}
	log.Printf("Listening on %v", l.Addr())
	return l
}

/* waitConn waits for a conn on the specified listener and sends on the chan */
func waitConn(l *net.TCPListener, ch chan *net.TCPConn, dead chan int) {
	for {
		c, err := l.AcceptTCP()
		if err != nil {
			log.Printf("Unable to accept connection on %v: %v",
				l.Addr(), err)
			close(ch)
			dead <- 1
			return
		}
		ch <- c
	}
}

/* waitDead waits for n ints on in, then sends to out */
func waitDead(n int, in, out chan int) {
	for i := 0; i < n; i++ {
		<-in
	}
	out <- 1
}

/* proxyBytes proxies bytes between src and dst using a buffer of buflen bytes.
it sends an int to done when it's done.  cstr describes the connection as
a string */
func proxyBytes(src, dst *net.TCPConn, done chan *net.TCPConn, buflen int,
	cstr string) {
	buf := make([]byte, buflen)
	read := 0
	written := 0
	rws := ""
	urws := func() {
		rws = fmt.Sprintf("%v read / %v written", read, written)
	}
	urws()
	for {
		/* Read a bit */
		n, err := src.Read(buf)
		read += n
		urws()
		if err != nil {
			/* End of file */
			if err == io.EOF {
				log.Printf("Connection ended (%v): %v", rws,
					cstr)
				done <- src
				break
			} else if e, ok := err.(*net.OpError); ok {
				if strings.HasSuffix(e.Error(),
					"use of closed network connection") {
					log.Printf("Connection closed (%v): "+
						"%v", rws, cstr)
				} else if strings.HasSuffix(e.Error(),
					"connection reset by peer") {
					log.Printf("Connection reset (%v): %v",
						rws, cstr)
				}
				break
			} else {
				log.Printf("Unknown error of type %T (%v): %v",
					err, rws, err)
				done <- src
				break
			}
		}
		/* TODO: Logging here */
		/* Write until it's done or an error happens */
		start := 0
		end := n
		for start < end {
			n, err := dst.Write(buf[start:end])
			written += n
			urws()
			if e, ok := err.(net.Error); ok {
				if !e.Temporary() {
					log.Printf("Error writing to %v: %v",
						cstr, err)
					done <- dst
					break
					/* TODO: Handle better */
				}
			}
			start += n
		}
	}
}
