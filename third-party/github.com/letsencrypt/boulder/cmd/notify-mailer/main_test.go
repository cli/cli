package notmain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"text/template"
	"time"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/mocks"
	"github.com/letsencrypt/boulder/test"
)

func TestIntervalOK(t *testing.T) {
	// Test a number of intervals know to be OK, ensure that no error is
	// produced when calling `ok()`.
	okCases := []struct {
		testInterval interval
	}{
		{interval{}},
		{interval{start: "aa", end: "\xFF"}},
		{interval{end: "aa"}},
		{interval{start: "aa", end: "bb"}},
	}
	for _, testcase := range okCases {
		err := testcase.testInterval.ok()
		test.AssertNotError(t, err, "valid interval produced ok() error")
	}

	badInterval := interval{start: "bb", end: "aa"}
	err := badInterval.ok()
	test.AssertError(t, err, "bad interval was considered ok")
}

func setupMakeRecipientList(t *testing.T, contents string) string {
	entryFile, err := os.CreateTemp("", "")
	test.AssertNotError(t, err, "couldn't create temp file")

	_, err = entryFile.WriteString(contents)
	test.AssertNotError(t, err, "couldn't write contents to temp file")

	err = entryFile.Close()
	test.AssertNotError(t, err, "couldn't close temp file")
	return entryFile.Name()
}

func TestReadRecipientList(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21
23,example.net,2018-11-22`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	list, _, err := readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")

	expected := []recipient{
		{id: 10, Data: map[string]string{"date": "2018-11-21", "domainName": "example.com"}},
		{id: 23, Data: map[string]string{"date": "2018-11-22", "domainName": "example.net"}},
	}
	test.AssertDeepEquals(t, list, expected)

	contents = `id	domainName	date
10	example.com	2018-11-21
23	example.net	2018-11-22`

	entryFile = setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	list, _, err = readRecipientsList(entryFile, '\t')
	test.AssertNotError(t, err, "received an error for a valid TSV file")
	test.AssertDeepEquals(t, list, expected)
}

func TestReadRecipientListNoExtraColumns(t *testing.T) {
	contents := `id
10
23`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")
}

func TestReadRecipientsListFileNoExist(t *testing.T) {
	_, _, err := readRecipientsList("doesNotExist", ',')
	test.AssertError(t, err, "expected error for a file that doesn't exist")
}

func TestReadRecipientListWithEmptyColumnInHeader(t *testing.T) {
	contents := `id, domainName,,date
10,example.com,2018-11-21
23,example.net`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "failed to error on CSV file with trailing delimiter in header")
	test.AssertDeepEquals(t, err, errors.New("header contains an empty column"))
}

func TestReadRecipientListWithProblems(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21
23,example.net,
10,example.com,2018-11-22
42,example.net,
24,example.com,2018-11-21
24,example.com,2018-11-21
`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	recipients, probs, err := readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")
	test.AssertEquals(t, probs, "ID(s) [23 42] contained empty columns and ID(s) [10 24] were skipped as duplicates")
	test.AssertEquals(t, len(recipients), 4)

	// Ensure trailing " and " is trimmed from single problem.
	contents = `id, domainName, date
23,example.net,
10,example.com,2018-11-21
42,example.net,
`

	entryFile = setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, probs, err = readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")
	test.AssertEquals(t, probs, "ID(s) [23 42] contained empty columns")
}

func TestReadRecipientListWithEmptyLine(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21

23,example.net,2018-11-22`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")
}

func TestReadRecipientListWithMismatchedColumns(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21
23,example.net`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "failed to error on CSV file with mismatched columns")
}

func TestReadRecipientListWithDuplicateIDs(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21
10,example.net,2018-11-22`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertNotError(t, err, "received an error for a valid CSV file")
}

func TestReadRecipientListWithUnparsableID(t *testing.T) {
	contents := `id, domainName, date
10,example.com,2018-11-21
twenty,example.net,2018-11-22`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "expected error for CSV file that contains an unparsable registration ID")
}

func TestReadRecipientListWithoutIDHeader(t *testing.T) {
	contents := `notId, domainName, date
10,example.com,2018-11-21
twenty,example.net,2018-11-22`

	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "expected error for CSV file missing header field `id`")
}

func TestReadRecipientListWithNoRecords(t *testing.T) {
	contents := `id, domainName, date
`
	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "expected error for CSV file containing only a header")
}

func TestReadRecipientListWithNoHeaderOrRecords(t *testing.T) {
	contents := ``
	entryFile := setupMakeRecipientList(t, contents)
	defer os.Remove(entryFile)

	_, _, err := readRecipientsList(entryFile, ',')
	test.AssertError(t, err, "expected error for CSV file containing only a header")
	test.AssertErrorIs(t, err, io.EOF)
}

func TestMakeMessageBody(t *testing.T) {
	emailTemplate := `{{range . }}
{{ .Data.date }}
{{ .Data.domainName }}
{{end}}`

	m := &mailer{
		log:           blog.UseMock(),
		mailer:        &mocks.Mailer{},
		emailTemplate: template.Must(template.New("email").Parse(emailTemplate)).Option("missingkey=error"),
		sleepInterval: 0,
		targetRange:   interval{end: "\xFF"},
		clk:           clock.NewFake(),
		recipients:    nil,
		dbMap:         mockEmailResolver{},
	}

	recipients := []recipient{
		{id: 10, Data: map[string]string{"date": "2018-11-21", "domainName": "example.com"}},
		{id: 23, Data: map[string]string{"date": "2018-11-22", "domainName": "example.net"}},
	}

	expectedMessageBody := `
2018-11-21
example.com

2018-11-22
example.net
`

	// Ensure that a very basic template with 2 recipients can be successfully
	// executed.
	messageBody, err := m.makeMessageBody(recipients)
	test.AssertNotError(t, err, "failed to execute a valid template")
	test.AssertEquals(t, messageBody, expectedMessageBody)

	// With no recipients we should get an empty body error.
	recipients = []recipient{}
	_, err = m.makeMessageBody(recipients)
	test.AssertError(t, err, "should have errored on empty body")

	// With a missing key we should get an informative templating error.
	recipients = []recipient{{id: 10, Data: map[string]string{"domainName": "example.com"}}}
	_, err = m.makeMessageBody(recipients)
	test.AssertEquals(t, err.Error(), "template: email:2:8: executing \"email\" at <.Data.date>: map has no entry for key \"date\"")
}

func TestSleepInterval(t *testing.T) {
	const sleepLen = 10
	mc := &mocks.Mailer{}
	dbMap := mockEmailResolver{}
	tmpl := template.Must(template.New("letter").Parse("an email body"))
	recipients := []recipient{{id: 1}, {id: 2}, {id: 3}}
	// Set up a mock mailer that sleeps for `sleepLen` seconds and only has one
	// goroutine to process results
	m := &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		emailTemplate: tmpl,
		sleepInterval: sleepLen * time.Second,
		parallelSends: 1,
		targetRange:   interval{start: "", end: "\xFF"},
		clk:           clock.NewFake(),
		recipients:    recipients,
		dbMap:         dbMap,
	}

	// Call run() - this should sleep `sleepLen` per destination address
	// After it returns, we expect (sleepLen * number of destinations) seconds has
	// elapsed
	err := m.run(context.Background())
	test.AssertNotError(t, err, "error calling mailer run()")
	expectedEnd := clock.NewFake()
	expectedEnd.Add(time.Second * time.Duration(sleepLen*len(recipients)))
	test.AssertEquals(t, m.clk.Now(), expectedEnd.Now())

	// Set up a mock mailer that doesn't sleep at all
	m = &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		emailTemplate: tmpl,
		sleepInterval: 0,
		targetRange:   interval{end: "\xFF"},
		clk:           clock.NewFake(),
		recipients:    recipients,
		dbMap:         dbMap,
	}

	// Call run() - this should blast through all destinations without sleep
	// After it returns, we expect no clock time to have elapsed on the fake clock
	err = m.run(context.Background())
	test.AssertNotError(t, err, "error calling mailer run()")
	expectedEnd = clock.NewFake()
	test.AssertEquals(t, m.clk.Now(), expectedEnd.Now())
}

func TestMailIntervals(t *testing.T) {
	const testSubject = "Test Subject"
	dbMap := mockEmailResolver{}

	tmpl := template.Must(template.New("letter").Parse("an email body"))
	recipients := []recipient{{id: 1}, {id: 2}, {id: 3}}

	mc := &mocks.Mailer{}

	// Create a mailer with a checkpoint interval larger than any of the
	// destination email addresses.
	m := &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    recipients,
		emailTemplate: tmpl,
		targetRange:   interval{start: "\xFF", end: "\xFF\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer. It should produce an error about the interval start
	mc.Clear()
	err := m.run(context.Background())
	test.AssertError(t, err, "expected error")
	test.AssertEquals(t, len(mc.Messages), 0)

	// Create a mailer with a negative sleep interval
	m = &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    recipients,
		emailTemplate: tmpl,
		targetRange:   interval{},
		sleepInterval: -10,
		clk:           clock.NewFake(),
	}

	// Run the mailer. It should produce an error about the sleep interval
	mc.Clear()
	err = m.run(context.Background())
	test.AssertEquals(t, len(mc.Messages), 0)
	test.AssertEquals(t, err.Error(), "sleep interval (-10) is < 0")

	// Create a mailer with an interval starting with a specific email address.
	// It should send email to that address and others alphabetically higher.
	m = &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    []recipient{{id: 1}, {id: 2}, {id: 3}, {id: 4}},
		emailTemplate: tmpl,
		targetRange:   interval{start: "test-example-updated@letsencrypt.org", end: "\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer. Two messages should have been produced, one to
	// test-example-updated@letsencrypt.org (beginning of the range),
	// and one to test-test-test@letsencrypt.org.
	mc.Clear()
	err = m.run(context.Background())
	test.AssertNotError(t, err, "run() produced an error")
	test.AssertEquals(t, len(mc.Messages), 2)
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "test-example-updated@letsencrypt.org",
		Subject: testSubject,
		Body:    "an email body",
	}, mc.Messages[0])
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "test-test-test@letsencrypt.org",
		Subject: testSubject,
		Body:    "an email body",
	}, mc.Messages[1])

	// Create a mailer with a checkpoint interval ending before
	// "test-example-updated@letsencrypt.org"
	m = &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    []recipient{{id: 1}, {id: 2}, {id: 3}, {id: 4}},
		emailTemplate: tmpl,
		targetRange:   interval{end: "test-example-updated@letsencrypt.org"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer. Two messages should have been produced, one to
	// example@letsencrypt.org (ID 1), one to example-example-example@example.com (ID 2)
	mc.Clear()
	err = m.run(context.Background())
	test.AssertNotError(t, err, "run() produced an error")
	test.AssertEquals(t, len(mc.Messages), 2)
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "example-example-example@letsencrypt.org",
		Subject: testSubject,
		Body:    "an email body",
	}, mc.Messages[0])
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "example@letsencrypt.org",
		Subject: testSubject,
		Body:    "an email body",
	}, mc.Messages[1])
}

func TestParallelism(t *testing.T) {
	const testSubject = "Test Subject"
	dbMap := mockEmailResolver{}

	tmpl := template.Must(template.New("letter").Parse("an email body"))
	recipients := []recipient{{id: 1}, {id: 2}, {id: 3}, {id: 4}}

	mc := &mocks.Mailer{}

	// Create a mailer with 10 parallel workers.
	m := &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    recipients,
		emailTemplate: tmpl,
		targetRange:   interval{end: "\xFF"},
		sleepInterval: 0,
		parallelSends: 10,
		clk:           clock.NewFake(),
	}

	mc.Clear()
	err := m.run(context.Background())
	test.AssertNotError(t, err, "run() produced an error")

	// The fake clock should have advanced 9 seconds, one for each parallel
	// goroutine after the first doing its polite 1-second sleep at startup.
	expectedEnd := clock.NewFake()
	expectedEnd.Add(9 * time.Second)
	test.AssertEquals(t, m.clk.Now(), expectedEnd.Now())

	// A message should have been sent to all four addresses.
	test.AssertEquals(t, len(mc.Messages), 4)
	expectedAddresses := []string{
		"example@letsencrypt.org",
		"test-example-updated@letsencrypt.org",
		"test-test-test@letsencrypt.org",
		"example-example-example@letsencrypt.org",
	}
	for _, msg := range mc.Messages {
		test.AssertSliceContains(t, expectedAddresses, msg.To)
	}
}

func TestMessageContentStatic(t *testing.T) {
	// Create a mailer with fixed content
	const (
		testSubject = "Test Subject"
	)
	dbMap := mockEmailResolver{}
	mc := &mocks.Mailer{}
	m := &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       testSubject,
		recipients:    []recipient{{id: 1}},
		emailTemplate: template.Must(template.New("letter").Parse("an email body")),
		targetRange:   interval{end: "\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer, one message should have been created with the content
	// expected
	err := m.run(context.Background())
	test.AssertNotError(t, err, "error calling mailer run()")
	test.AssertEquals(t, len(mc.Messages), 1)
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "example@letsencrypt.org",
		Subject: testSubject,
		Body:    "an email body",
	}, mc.Messages[0])
}

// Send mail with a variable interpolated.
func TestMessageContentInterpolated(t *testing.T) {
	recipients := []recipient{
		{
			id: 1,
			Data: map[string]string{
				"validationMethod": "eyeballing it",
			},
		},
	}
	dbMap := mockEmailResolver{}
	mc := &mocks.Mailer{}
	m := &mailer{
		log:        blog.UseMock(),
		mailer:     mc,
		dbMap:      dbMap,
		subject:    "Test Subject",
		recipients: recipients,
		emailTemplate: template.Must(template.New("letter").Parse(
			`issued by {{range .}}{{ .Data.validationMethod }}{{end}}`)),
		targetRange:   interval{end: "\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer, one message should have been created with the content
	// expected
	err := m.run(context.Background())
	test.AssertNotError(t, err, "error calling mailer run()")
	test.AssertEquals(t, len(mc.Messages), 1)
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "example@letsencrypt.org",
		Subject: "Test Subject",
		Body:    "issued by eyeballing it",
	}, mc.Messages[0])
}

// Send mail with a variable interpolated multiple times for accounts that share
// an email address.
func TestMessageContentInterpolatedMultiple(t *testing.T) {
	recipients := []recipient{
		{
			id: 200,
			Data: map[string]string{
				"domain": "blog.example.com",
			},
		},
		{
			id: 201,
			Data: map[string]string{
				"domain": "nas.example.net",
			},
		},
		{
			id: 202,
			Data: map[string]string{
				"domain": "mail.example.org",
			},
		},
		{
			id: 203,
			Data: map[string]string{
				"domain": "panel.example.net",
			},
		},
	}
	dbMap := mockEmailResolver{}
	mc := &mocks.Mailer{}
	m := &mailer{
		log:        blog.UseMock(),
		mailer:     mc,
		dbMap:      dbMap,
		subject:    "Test Subject",
		recipients: recipients,
		emailTemplate: template.Must(template.New("letter").Parse(
			`issued for:
{{range .}}{{ .Data.domain }}
{{end}}Thanks`)),
		targetRange:   interval{end: "\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	// Run the mailer, one message should have been created with the content
	// expected
	err := m.run(context.Background())
	test.AssertNotError(t, err, "error calling mailer run()")
	test.AssertEquals(t, len(mc.Messages), 1)
	test.AssertEquals(t, mocks.MailerMessage{
		To:      "gotta.lotta.accounts@letsencrypt.org",
		Subject: "Test Subject",
		Body: `issued for:
blog.example.com
nas.example.net
mail.example.org
panel.example.net
Thanks`,
	}, mc.Messages[0])
}

// the `mockEmailResolver` implements the `dbSelector` interface from
// `notify-mailer/main.go` to allow unit testing without using a backing
// database
type mockEmailResolver struct{}

// the `mockEmailResolver` select method treats the requested reg ID as an index
// into a list of anonymous structs
func (bs mockEmailResolver) SelectOne(ctx context.Context, output interface{}, _ string, args ...interface{}) error {
	// The "dbList" is just a list of contact records in memory
	dbList := []contactQueryResult{
		{
			ID:      1,
			Contact: []byte(`["mailto:example@letsencrypt.org"]`),
		},
		{
			ID:      2,
			Contact: []byte(`["mailto:test-example-updated@letsencrypt.org"]`),
		},
		{
			ID:      3,
			Contact: []byte(`["mailto:test-test-test@letsencrypt.org"]`),
		},
		{
			ID:      4,
			Contact: []byte(`["mailto:example-example-example@letsencrypt.org"]`),
		},
		{
			ID:      5,
			Contact: []byte(`["mailto:youve.got.mail@letsencrypt.org"]`),
		},
		{
			ID:      6,
			Contact: []byte(`["mailto:mail@letsencrypt.org"]`),
		},
		{
			ID:      7,
			Contact: []byte(`["mailto:***********"]`),
		},
		{
			ID:      200,
			Contact: []byte(`["mailto:gotta.lotta.accounts@letsencrypt.org"]`),
		},
		{
			ID:      201,
			Contact: []byte(`["mailto:gotta.lotta.accounts@letsencrypt.org"]`),
		},
		{
			ID:      202,
			Contact: []byte(`["mailto:gotta.lotta.accounts@letsencrypt.org"]`),
		},
		{
			ID:      203,
			Contact: []byte(`["mailto:gotta.lotta.accounts@letsencrypt.org"]`),
		},
		{
			ID:      204,
			Contact: []byte(`["mailto:gotta.lotta.accounts@letsencrypt.org"]`),
		},
	}

	// Play the type cast game so that we can dig into the arguments map and get
	// out an int64 `id` parameter.
	argsRaw := args[0]
	argsMap, ok := argsRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("incorrect args type %T", args)
	}
	idRaw := argsMap["id"]
	id, ok := idRaw.(int64)
	if !ok {
		return fmt.Errorf("incorrect args ID type %T", id)
	}

	// Play the type cast game to get a `*contactQueryResult` so we can write
	// the result from the db list.
	outputPtr, ok := output.(*contactQueryResult)
	if !ok {
		return fmt.Errorf("incorrect output type %T", output)
	}

	for _, v := range dbList {
		if v.ID == id {
			*outputPtr = v
		}
	}
	if outputPtr.ID == 0 {
		return db.ErrDatabaseOp{
			Op:    "select one",
			Table: "registrations",
			Err:   sql.ErrNoRows,
		}
	}
	return nil
}

func TestResolveEmails(t *testing.T) {
	// Start with three reg. IDs. Note: the IDs have been matched with fake
	// results in the `db` slice in `mockEmailResolver`'s `SelectOne`. If you add
	// more test cases here you must also add the corresponding DB result in the
	// mock.
	recipients := []recipient{
		{
			id: 1,
		},
		{
			id: 2,
		},
		{
			id: 3,
		},
		// This registration ID deliberately doesn't exist in the mock data to make
		// sure this case is handled gracefully
		{
			id: 999,
		},
		// This registration ID deliberately returns an invalid email to make sure any
		// invalid contact info that slipped into the DB once upon a time will be ignored
		{
			id: 7,
		},
		{
			id: 200,
		},
		{
			id: 201,
		},
		{
			id: 202,
		},
		{
			id: 203,
		},
		{
			id: 204,
		},
	}

	tmpl := template.Must(template.New("letter").Parse("an email body"))

	dbMap := mockEmailResolver{}
	mc := &mocks.Mailer{}
	m := &mailer{
		log:           blog.UseMock(),
		mailer:        mc,
		dbMap:         dbMap,
		subject:       "Test",
		recipients:    recipients,
		emailTemplate: tmpl,
		targetRange:   interval{end: "\xFF"},
		sleepInterval: 0,
		clk:           clock.NewFake(),
	}

	addressesToRecipients, err := m.resolveAddresses(context.Background())
	test.AssertNotError(t, err, "failed to resolveEmailAddresses")

	expected := []string{
		"example@letsencrypt.org",
		"test-example-updated@letsencrypt.org",
		"test-test-test@letsencrypt.org",
		"gotta.lotta.accounts@letsencrypt.org",
	}

	test.AssertEquals(t, len(addressesToRecipients), len(expected))
	for _, address := range expected {
		if _, ok := addressesToRecipients[address]; !ok {
			t.Errorf("missing entry in addressesToRecipients: %q", address)
		}
	}
}
