package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// filter filters mails based on the To: and From: fields.
// The zero value matches all mails.
type filter struct {
	To   string
	From string
}

func (f *filter) Match(m rcvdMail) bool {
	if f.To != "" && f.To != m.To {
		return false
	}
	if f.From != "" && f.From != m.From {
		return false
	}
	return true
}

/*
/count - number of mails
/count?to=foo@bar.com - number of mails for foo@bar.com
/count?from=service@test.org - number of mails sent by service@test.org
/clear - clear the mail list
/mail/0 - first mail
/mail/1 - second mail
/mail/0?to=foo@bar.com - first mail for foo@bar.com
/mail/1?to=foo@bar.com - second mail for foo@bar.com
/mail/1?to=foo@bar.com&from=service@test.org - second mail for foo@bar.com from service@test.org
*/

func (srv *mailSrv) setupHTTP(serveMux *http.ServeMux) {
	serveMux.HandleFunc("/count", srv.httpCount)
	serveMux.HandleFunc("/clear", srv.httpClear)
	serveMux.Handle("/mail/", http.StripPrefix("/mail/", http.HandlerFunc(srv.httpGetMail)))
}

func (srv *mailSrv) httpClear(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		srv.allMailMutex.Lock()
		srv.allReceivedMail = nil
		srv.allMailMutex.Unlock()
		w.WriteHeader(200)
	} else {
		w.WriteHeader(405)
	}
}

func (srv *mailSrv) httpCount(w http.ResponseWriter, r *http.Request) {
	count := 0
	srv.iterMail(extractFilter(r), func(m rcvdMail) bool {
		count++
		return false
	})
	fmt.Fprintf(w, "%d\n", count)
}

func (srv *mailSrv) httpGetMail(w http.ResponseWriter, r *http.Request) {
	mailNum, err := strconv.Atoi(strings.Trim(r.URL.Path, "/"))
	if err != nil {
		w.WriteHeader(400)
		log.Println("mail-test-srv: bad request:", r.URL.Path, "-", err)
		return
	}
	idx := 0
	found := srv.iterMail(extractFilter(r), func(m rcvdMail) bool {
		if mailNum == idx {
			printMail(w, m)
			return true
		}
		idx++
		return false
	})
	if !found {
		w.WriteHeader(404)
	}
}

func extractFilter(r *http.Request) filter {
	values := r.URL.Query()
	return filter{To: values.Get("to"), From: values.Get("from")}
}

func (srv *mailSrv) iterMail(f filter, cb func(rcvdMail) bool) bool {
	srv.allMailMutex.Lock()
	defer srv.allMailMutex.Unlock()
	for _, v := range srv.allReceivedMail {
		if !f.Match(v) {
			continue
		}
		if cb(v) {
			return true
		}
	}
	return false
}

func printMail(w io.Writer, mail rcvdMail) {
	fmt.Fprintf(w, "FROM %s\n", mail.From)
	fmt.Fprintf(w, "TO %s\n", mail.To)
	fmt.Fprintf(w, "\n%s\n", mail.Mail)
}
