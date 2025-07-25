package validator

import (
	"fmt"

	"github.com/letsencrypt/boulder/log"
)

// tailLogger is an adapter to the nxadm/tail module's logging interface.
type tailLogger struct {
	log.Logger
}

func (tl tailLogger) Fatal(v ...interface{}) {
	tl.AuditErr(fmt.Sprint(v...))
}
func (tl tailLogger) Fatalf(format string, v ...interface{}) {
	tl.AuditErrf(format, v...)
}
func (tl tailLogger) Fatalln(v ...interface{}) {
	tl.AuditErr(fmt.Sprint(v...) + "\n")
}
func (tl tailLogger) Panic(v ...interface{}) {
	tl.AuditErr(fmt.Sprint(v...))
}
func (tl tailLogger) Panicf(format string, v ...interface{}) {
	tl.AuditErrf(format, v...)
}
func (tl tailLogger) Panicln(v ...interface{}) {
	tl.AuditErr(fmt.Sprint(v...) + "\n")
}
func (tl tailLogger) Print(v ...interface{}) {
	tl.Info(fmt.Sprint(v...))
}
func (tl tailLogger) Printf(format string, v ...interface{}) {
	tl.Infof(format, v...)
}
func (tl tailLogger) Println(v ...interface{}) {
	tl.Info(fmt.Sprint(v...) + "\n")
}
