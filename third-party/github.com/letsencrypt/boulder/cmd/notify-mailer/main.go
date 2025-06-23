package notmain

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	bmail "github.com/letsencrypt/boulder/mail"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/sa"
)

type mailer struct {
	clk           clock.Clock
	log           blog.Logger
	dbMap         dbSelector
	mailer        bmail.Mailer
	subject       string
	emailTemplate *template.Template
	recipients    []recipient
	targetRange   interval
	sleepInterval time.Duration
	parallelSends uint
}

// interval defines a range of email addresses to send to in alphabetical order.
// The `start` field is inclusive and the `end` field is exclusive. To include
// everything, set `end` to \xFF.
type interval struct {
	start string
	end   string
}

// contactQueryResult is a receiver for queries to the `registrations` table.
type contactQueryResult struct {
	// ID is exported to receive the value of `id`.
	ID int64

	// Contact is exported to receive the value of `contact`.
	Contact []byte
}

func (i *interval) ok() error {
	if i.start > i.end {
		return fmt.Errorf("interval start value (%s) is greater than end value (%s)",
			i.start, i.end)
	}
	return nil
}

func (i *interval) includes(s string) bool {
	return s >= i.start && s < i.end
}

// ok ensures that both the `targetRange` and `sleepInterval` are valid.
func (m *mailer) ok() error {
	err := m.targetRange.ok()
	if err != nil {
		return err
	}

	if m.sleepInterval < 0 {
		return fmt.Errorf(
			"sleep interval (%d) is < 0", m.sleepInterval)
	}
	return nil
}

func (m *mailer) logStatus(to string, current, total int, start time.Time) {
	// Should never happen.
	if total <= 0 || current < 1 || current > total {
		m.log.AuditErrf("Invalid current (%d) or total (%d)", current, total)
	}
	completion := (float32(current) / float32(total)) * 100
	now := m.clk.Now()
	elapsed := now.Sub(start)
	m.log.Infof("Sending message (%d) of (%d) to address (%s) [%.2f%%] time elapsed (%s)",
		current, total, to, completion, elapsed)
}

func sortAddresses(input addressToRecipientMap) []string {
	var addresses []string
	for address := range input {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
}

// makeMessageBody is a helper for mailer.run() that's split out for the
// purposes of testing.
func (m *mailer) makeMessageBody(recipients []recipient) (string, error) {
	var messageBody strings.Builder

	err := m.emailTemplate.Execute(&messageBody, recipients)
	if err != nil {
		return "", err
	}

	if messageBody.Len() == 0 {
		return "", errors.New("templating resulted in an empty message body")
	}
	return messageBody.String(), nil
}

func (m *mailer) run(ctx context.Context) error {
	err := m.ok()
	if err != nil {
		return err
	}

	totalRecipients := len(m.recipients)
	m.log.Infof("Resolving addresses for (%d) recipients", totalRecipients)

	addressToRecipient, err := m.resolveAddresses(ctx)
	if err != nil {
		return err
	}

	totalAddresses := len(addressToRecipient)
	if totalAddresses == 0 {
		return errors.New("0 recipients remained after resolving addresses")
	}

	m.log.Infof("%d recipients were resolved to %d addresses", totalRecipients, totalAddresses)

	var mostRecipients string
	var mostRecipientsLen int
	for k, v := range addressToRecipient {
		if len(v) > mostRecipientsLen {
			mostRecipientsLen = len(v)
			mostRecipients = k
		}
	}

	m.log.Infof("Address %q was associated with the most recipients (%d)",
		mostRecipients, mostRecipientsLen)

	type work struct {
		index   int
		address string
	}

	var wg sync.WaitGroup
	workChan := make(chan work, totalAddresses)

	startTime := m.clk.Now()
	sortedAddresses := sortAddresses(addressToRecipient)

	if (m.targetRange.start != "" && m.targetRange.start > sortedAddresses[totalAddresses-1]) ||
		(m.targetRange.end != "" && m.targetRange.end < sortedAddresses[0]) {
		return errors.New("Zero found addresses fall inside target range")
	}

	go func(ch chan<- work) {
		for i, address := range sortedAddresses {
			ch <- work{i, address}
		}
		close(workChan)
	}(workChan)

	if m.parallelSends < 1 {
		m.parallelSends = 1
	}

	for senderNum := uint(0); senderNum < m.parallelSends; senderNum++ {
		// For politeness' sake, don't open more than 1 new connection per
		// second.
		if senderNum > 0 {
			m.clk.Sleep(time.Second)
		}

		conn, err := m.mailer.Connect()
		if err != nil {
			return fmt.Errorf("connecting parallel sender %d: %w", senderNum, err)
		}

		wg.Add(1)
		go func(conn bmail.Conn, ch <-chan work) {
			defer wg.Done()
			for w := range ch {
				if !m.targetRange.includes(w.address) {
					m.log.Debugf("Address %q is outside of target range, skipping", w.address)
					continue
				}

				err := policy.ValidEmail(w.address)
				if err != nil {
					m.log.Infof("Skipping %q due to policy violation: %s", w.address, err)
					continue
				}

				recipients := addressToRecipient[w.address]
				m.logStatus(w.address, w.index+1, totalAddresses, startTime)

				messageBody, err := m.makeMessageBody(recipients)
				if err != nil {
					m.log.Errf("Skipping %q due to templating error: %s", w.address, err)
					continue
				}

				err = conn.SendMail([]string{w.address}, m.subject, messageBody)
				if err != nil {
					var badAddrErr bmail.BadAddressSMTPError
					if errors.As(err, &badAddrErr) {
						m.log.Errf("address %q was rejected by server: %s", w.address, err)
						continue
					}
					m.log.AuditErrf("while sending mail (%d) of (%d) to address %q: %s",
						w.index, len(sortedAddresses), w.address, err)
				}

				m.clk.Sleep(m.sleepInterval)
			}
			conn.Close()
		}(conn, workChan)
	}
	wg.Wait()

	return nil
}

// resolveAddresses creates a mapping of email addresses to (a list of)
// `recipient`s that resolve to that email address.
func (m *mailer) resolveAddresses(ctx context.Context) (addressToRecipientMap, error) {
	result := make(addressToRecipientMap, len(m.recipients))
	for _, recipient := range m.recipients {
		addresses, err := getAddressForID(ctx, recipient.id, m.dbMap)
		if err != nil {
			return nil, err
		}

		for _, address := range addresses {
			parsed, err := mail.ParseAddress(address)
			if err != nil {
				m.log.Errf("Unparsable address %q, skipping ID (%d)", address, recipient.id)
				continue
			}
			result[parsed.Address] = append(result[parsed.Address], recipient)
		}
	}
	return result, nil
}

// dbSelector abstracts over a subset of methods from `borp.DbMap` objects to
// facilitate mocking in unit tests.
type dbSelector interface {
	SelectOne(ctx context.Context, holder interface{}, query string, args ...interface{}) error
}

// getAddressForID queries the database for the email address associated with
// the provided registration ID.
func getAddressForID(ctx context.Context, id int64, dbMap dbSelector) ([]string, error) {
	var result contactQueryResult
	err := dbMap.SelectOne(ctx, &result,
		`SELECT id,
			contact
		FROM registrations
		WHERE contact NOT IN ('[]', 'null')
			AND id = :id;`,
		map[string]interface{}{"id": id})
	if err != nil {
		if db.IsNoRows(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var contacts []string
	err = json.Unmarshal(result.Contact, &contacts)
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, contact := range contacts {
		if strings.HasPrefix(contact, "mailto:") {
			addresses = append(addresses, strings.TrimPrefix(contact, "mailto:"))
		}
	}
	return addresses, nil
}

// recipient represents a single record from the recipient list file. The 'id'
// column is parsed to the 'id' field, all additional data will be parsed to a
// mapping of column name to value in the 'Data' field. Please inform SRE if you
// make any changes to the exported fields of this struct. These fields are
// referenced in operationally critical e-mail templates used to notify
// subscribers during incident response.
type recipient struct {
	// id is the subscriber's ID.
	id int64

	// Data is a mapping of column name to value parsed from a single record in
	// the provided recipient list file. It's exported so the contents can be
	// accessed by the template package. Please inform SRE if you make any
	// changes to this field.
	Data map[string]string
}

// addressToRecipientMap maps email addresses to a list of `recipient`s that
// resolve to that email address.
type addressToRecipientMap map[string][]recipient

// readRecipientsList parses the contents of a recipient list file into a list
// of `recipient` objects.
func readRecipientsList(filename string, delimiter rune) ([]recipient, string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}

	reader := csv.NewReader(f)
	reader.Comma = delimiter

	// Parse header.
	record, err := reader.Read()
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse header: %w", err)
	}

	if record[0] != "id" {
		return nil, "", errors.New("header must begin with \"id\"")
	}

	// Collect the names of each header column after `id`.
	var dataColumns []string
	for _, v := range record[1:] {
		dataColumns = append(dataColumns, strings.TrimSpace(v))
		if len(v) == 0 {
			return nil, "", errors.New("header contains an empty column")
		}
	}

	var recordsWithEmptyColumns []int64
	var recordsWithDuplicateIDs []int64
	var probsBuff strings.Builder
	stringProbs := func() string {
		if len(recordsWithEmptyColumns) != 0 {
			fmt.Fprintf(&probsBuff, "ID(s) %v contained empty columns and ",
				recordsWithEmptyColumns)
		}

		if len(recordsWithDuplicateIDs) != 0 {
			fmt.Fprintf(&probsBuff, "ID(s) %v were skipped as duplicates",
				recordsWithDuplicateIDs)
		}

		if probsBuff.Len() == 0 {
			return ""
		}
		return strings.TrimSuffix(probsBuff.String(), " and ")
	}

	// Parse records.
	recipientIDs := make(map[int64]bool)
	var recipients []recipient
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			// Finished parsing the file.
			if len(recipients) == 0 {
				return nil, stringProbs(), errors.New("no records after header")
			}
			return recipients, stringProbs(), nil
		} else if err != nil {
			return nil, "", err
		}

		// Ensure the first column of each record can be parsed as a valid
		// registration ID.
		recordID := record[0]
		id, err := strconv.ParseInt(recordID, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf(
				"%q couldn't be parsed as a registration ID due to: %s", recordID, err)
		}

		// Skip records that have the same ID as those read previously.
		if recipientIDs[id] {
			recordsWithDuplicateIDs = append(recordsWithDuplicateIDs, id)
			continue
		}
		recipientIDs[id] = true

		// Collect the columns of data after `id` into a map.
		var emptyColumn bool
		data := make(map[string]string)
		for i, v := range record[1:] {
			if len(v) == 0 {
				emptyColumn = true
			}
			data[dataColumns[i]] = v
		}

		// Only used for logging.
		if emptyColumn {
			recordsWithEmptyColumns = append(recordsWithEmptyColumns, id)
		}

		recipients = append(recipients, recipient{id, data})
	}
}

const usageIntro = `
Introduction:

The notification mailer exists to send a message to the contact associated
with a list of registration IDs. The attributes of the message (from address,
subject, and message content) are provided by the command line arguments. The
message content is provided as a path to a template file via the -body argument.

Provide a list of recipient user ids in a CSV file passed with the -recipientList
flag. The CSV file must have "id" as the first column and may have additional
fields to be interpolated into the email template:

	id, lastIssuance
	1234, "from example.com 2018-12-01"
	5678, "from example.net 2018-12-13"

The additional fields will be interpolated with Golang templating, e.g.:

  Your last issuance on each account was:
		{{ range . }} {{ .Data.lastIssuance }}
		{{ end }}

To help the operator gain confidence in the mailing run before committing fully
three safety features are supported: dry runs, intervals and a sleep between emails.

The -dryRun=true flag will use a mock mailer that prints message content to
stdout instead of performing an SMTP transaction with a real mailserver. This
can be used when the initial parameters are being tweaked to ensure no real
emails are sent. Using -dryRun=false will send real email.

Intervals supported via the -start and -end arguments. Only email addresses that
are alphabetically between the -start and -end strings will be sent. This can be used
to break up sending into batches, or more likely to resume sending if a batch is killed,
without resending messages that have already been sent. The -start flag is inclusive and
the -end flag is exclusive.

Notify-mailer de-duplicates email addresses and groups together the resulting recipient
structs, so a person who has multiple accounts using the same address will only receive
one email.

During mailing the -sleep argument is used to space out individual messages.
This can be used to ensure that the mailing happens at a steady pace with ample
opportunity for the operator to terminate early in the event of error. The
-sleep flag honours durations with a unit suffix (e.g. 1m for 1 minute, 10s for
10 seconds, etc). Using -sleep=0 will disable the sleep and send at full speed.

Examples:
  Send an email with subject "Hello!" from the email "hello@goodbye.com" with
  the contents read from "test_msg_body.txt" to every email associated with the
  registration IDs listed in "test_reg_recipients.json", sleeping 10 seconds
  between each message:

  notify-mailer -config test/config/notify-mailer.json -body
    cmd/notify-mailer/testdata/test_msg_body.txt -from hello@goodbye.com
    -recipientList cmd/notify-mailer/testdata/test_msg_recipients.csv -subject "Hello!"
    -sleep 10s -dryRun=false

  Do the same, but only to example@example.com:

  notify-mailer -config test/config/notify-mailer.json
    -body cmd/notify-mailer/testdata/test_msg_body.txt -from hello@goodbye.com
    -recipientList cmd/notify-mailer/testdata/test_msg_recipients.csv -subject "Hello!"
    -start example@example.com -end example@example.comX

  Send the message starting with example@example.com and emailing every address that's
	alphabetically higher:

  notify-mailer -config test/config/notify-mailer.json 
    -body cmd/notify-mailer/testdata/test_msg_body.txt -from hello@goodbye.com 
    -recipientList cmd/notify-mailer/testdata/test_msg_recipients.csv -subject "Hello!"
    -start example@example.com

Required arguments:
- body
- config
- from
- subject
- recipientList`

type Config struct {
	NotifyMailer struct {
		DB cmd.DBConfig
		cmd.SMTPConfig
	}
	Syslog cmd.SyslogConfig
}

func main() {
	from := flag.String("from", "", "From header for emails. Must be a bare email address.")
	subject := flag.String("subject", "", "Subject of emails")
	recipientListFile := flag.String("recipientList", "", "File containing a CSV list of registration IDs and extra info.")
	parseAsTSV := flag.Bool("tsv", false, "Parse the recipient list file as a TSV.")
	bodyFile := flag.String("body", "", "File containing the email body in Golang template format.")
	dryRun := flag.Bool("dryRun", true, "Whether to do a dry run.")
	sleep := flag.Duration("sleep", 500*time.Millisecond, "How long to sleep between emails.")
	parallelSends := flag.Uint("parallelSends", 1, "How many parallel goroutines should process emails")
	start := flag.String("start", "", "Alphabetically lowest email address to include.")
	end := flag.String("end", "\xFF", "Alphabetically highest email address (exclusive).")
	reconnBase := flag.Duration("reconnectBase", 1*time.Second, "Base sleep duration between reconnect attempts")
	reconnMax := flag.Duration("reconnectMax", 5*60*time.Second, "Max sleep duration between reconnect attempts after exponential backoff")
	configFile := flag.String("config", "", "File containing a JSON config.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\n", usageIntro)
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	// Validate required args.
	flag.Parse()
	if *from == "" || *subject == "" || *bodyFile == "" || *configFile == "" || *recipientListFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	configData, err := os.ReadFile(*configFile)
	cmd.FailOnError(err, "Couldn't load JSON config file")

	// Parse JSON config.
	var cfg Config
	err = json.Unmarshal(configData, &cfg)
	cmd.FailOnError(err, "Couldn't unmarshal JSON config file")

	log := cmd.NewLogger(cfg.Syslog)
	log.Info(cmd.VersionString())

	dbMap, err := sa.InitWrappedDb(cfg.NotifyMailer.DB, nil, log)
	cmd.FailOnError(err, "While initializing dbMap")

	// Load and parse message body.
	template, err := template.ParseFiles(*bodyFile)
	cmd.FailOnError(err, "Couldn't parse message template")

	// Ensure that in the event of a missing key, an informative error is
	// returned.
	template.Option("missingkey=error")

	address, err := mail.ParseAddress(*from)
	cmd.FailOnError(err, fmt.Sprintf("Couldn't parse %q to address", *from))

	recipientListDelimiter := ','
	if *parseAsTSV {
		recipientListDelimiter = '\t'
	}
	recipients, probs, err := readRecipientsList(*recipientListFile, recipientListDelimiter)
	cmd.FailOnError(err, "Couldn't populate recipients")

	if probs != "" {
		log.Infof("While reading the recipient list file %s", probs)
	}

	var mailClient bmail.Mailer
	if *dryRun {
		log.Infof("Starting %s in dry-run mode", cmd.VersionString())
		mailClient = bmail.NewDryRun(*address, log)
	} else {
		log.Infof("Starting %s", cmd.VersionString())
		smtpPassword, err := cfg.NotifyMailer.PasswordConfig.Pass()
		cmd.FailOnError(err, "Couldn't load SMTP password from file")

		mailClient = bmail.New(
			cfg.NotifyMailer.Server,
			cfg.NotifyMailer.Port,
			cfg.NotifyMailer.Username,
			smtpPassword,
			nil,
			*address,
			log,
			metrics.NoopRegisterer,
			*reconnBase,
			*reconnMax)
	}

	m := mailer{
		clk:           cmd.Clock(),
		log:           log,
		dbMap:         dbMap,
		mailer:        mailClient,
		subject:       *subject,
		recipients:    recipients,
		emailTemplate: template,
		targetRange: interval{
			start: *start,
			end:   *end,
		},
		sleepInterval: *sleep,
		parallelSends: *parallelSends,
	}

	err = m.run(context.TODO())
	cmd.FailOnError(err, "Couldn't complete")

	log.Info("Completed successfully")
}

func init() {
	cmd.RegisterCommand("notify-mailer", main, &cmd.ConfigValidator{Config: &Config{}})
}
