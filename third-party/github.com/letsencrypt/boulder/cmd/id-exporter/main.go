package notmain

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/db"
	"github.com/letsencrypt/boulder/features"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/sa"
)

type idExporter struct {
	log   blog.Logger
	dbMap *db.WrappedMap
	clk   clock.Clock
	grace time.Duration
}

// resultEntry is a JSON marshalable exporter result entry.
type resultEntry struct {
	// ID is exported to support marshaling to JSON.
	ID int64 `json:"id"`

	// Hostname is exported to support marshaling to JSON. Not all queries
	// will fill this field, so it's JSON field tag marks at as
	// omittable.
	Hostname string `json:"hostname,omitempty"`
}

// reverseHostname converts (reversed) names sourced from the
// registrations table to standard hostnames.
func (r *resultEntry) reverseHostname() {
	r.Hostname = sa.ReverseName(r.Hostname)
}

// idExporterResults is passed as a selectable 'holder' for the results
// of id-exporter database queries
type idExporterResults []*resultEntry

// marshalToJSON returns JSON as bytes for all elements of the inner `id`
// slice.
func (i *idExporterResults) marshalToJSON() ([]byte, error) {
	data, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

// writeToFile writes the contents of the inner `ids` slice, as JSON, to
// a file
func (i *idExporterResults) writeToFile(outfile string) error {
	data, err := i.marshalToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(outfile, data, 0644)
}

// findIDs gathers all registration IDs with unexpired certificates.
func (c idExporter) findIDs(ctx context.Context) (idExporterResults, error) {
	var holder idExporterResults
	_, err := c.dbMap.Select(
		ctx,
		&holder,
		`SELECT DISTINCT r.id
		FROM registrations AS r
			INNER JOIN certificates AS c on c.registrationID = r.id
		WHERE r.contact NOT IN ('[]', 'null')
			AND c.expires >= :expireCutoff;`,
		map[string]interface{}{
			"expireCutoff": c.clk.Now().Add(-c.grace),
		})
	if err != nil {
		c.log.AuditErrf("Error finding IDs: %s", err)
		return nil, err
	}
	return holder, nil
}

// findIDsWithExampleHostnames gathers all registration IDs with
// unexpired certificates and a corresponding example hostname.
func (c idExporter) findIDsWithExampleHostnames(ctx context.Context) (idExporterResults, error) {
	var holder idExporterResults
	_, err := c.dbMap.Select(
		ctx,
		&holder,
		`SELECT SQL_BIG_RESULT
			cert.registrationID AS id,
			name.reversedName AS hostname
		FROM certificates AS cert
			INNER JOIN issuedNames AS name ON name.serial = cert.serial
		WHERE cert.expires >= :expireCutoff
		GROUP BY cert.registrationID;`,
		map[string]interface{}{
			"expireCutoff": c.clk.Now().Add(-c.grace),
		})
	if err != nil {
		c.log.AuditErrf("Error finding IDs and example hostnames: %s", err)
		return nil, err
	}

	for _, result := range holder {
		result.reverseHostname()
	}
	return holder, nil
}

// findIDsForHostnames gathers all registration IDs with unexpired
// certificates for each `hostnames` entry.
func (c idExporter) findIDsForHostnames(ctx context.Context, hostnames []string) (idExporterResults, error) {
	var holder idExporterResults
	for _, hostname := range hostnames {
		// Pass the same list in each time, borp will happily just append to the slice
		// instead of overwriting it each time
		// https://github.com/letsencrypt/borp/blob/c87bd6443d59746a33aca77db34a60cfc344adb2/select.go#L349-L353
		_, err := c.dbMap.Select(
			ctx,
			&holder,
			`SELECT DISTINCT c.registrationID AS id
			FROM certificates AS c
				INNER JOIN issuedNames AS n ON c.serial = n.serial
			WHERE c.expires >= :expireCutoff
				AND n.reversedName = :reversedName;`,
			map[string]interface{}{
				"expireCutoff": c.clk.Now().Add(-c.grace),
				"reversedName": sa.ReverseName(hostname),
			},
		)
		if err != nil {
			if db.IsNoRows(err) {
				continue
			}
			return nil, err
		}
	}

	return holder, nil
}

const usageIntro = `
Introduction:

The ID exporter exists to retrieve the IDs of all registered
users with currently unexpired certificates. This list of registration IDs can
then be given as input to the notification mailer to send bulk notifications.

The -grace parameter can be used to allow registrations with certificates that
have already expired to be included in the export. The argument is a Go duration
obeying the usual suffix rules (e.g. 24h).

Registration IDs are favoured over email addresses as the intermediate format in
order to ensure the most up to date contact information is used at the time of
notification. The notification mailer will resolve the ID to email(s) when the
mailing is underway, ensuring we use the correct address if a user has updated
their contact information between the time of export and the time of
notification.

By default, the ID exporter's output will be JSON of the form:
  [
    { "id": 1 },
    ...
    { "id": n }
  ]

Operations that return a hostname will be JSON of the form:
  [
    { "id": 1, "hostname": "example-1.com" },
    ...
    { "id": n, "hostname": "example-n.com" }
  ]

Examples:
  Export all registration IDs with unexpired certificates to "regs.json":

  id-exporter -config test/config/id-exporter.json -outfile regs.json

  Export all registration IDs with certificates that are unexpired or expired
  within the last two days to "regs.json":

  id-exporter -config test/config/id-exporter.json -grace 48h -outfile
    "regs.json"

Required arguments:
- config
- outfile`

// unmarshalHostnames unmarshals a hostnames file and ensures that the file
// contained at least one entry.
func unmarshalHostnames(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var hostnames []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, " ") {
			return nil, fmt.Errorf(
				"line: %q contains more than one entry, entries must be separated by newlines", line)
		}
		hostnames = append(hostnames, line)
	}

	if len(hostnames) == 0 {
		return nil, errors.New("provided file contains 0 hostnames")
	}
	return hostnames, nil
}

type Config struct {
	ContactExporter struct {
		DB cmd.DBConfig
		cmd.PasswordConfig
		Features features.Config
	}
}

func main() {
	outFile := flag.String("outfile", "", "File to output results JSON to.")
	grace := flag.Duration("grace", 2*24*time.Hour, "Include results with certificates that expired in < grace ago.")
	hostnamesFile := flag.String(
		"hostnames", "", "Only include results with unexpired certificates that contain hostnames\nlisted (newline separated) in this file.")
	withExampleHostnames := flag.Bool(
		"with-example-hostnames", false, "Include an example hostname for each registration ID with an unexpired certificate.")
	configFile := flag.String("config", "", "File containing a JSON config.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\n", usageIntro)
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	// Parse flags and check required.
	flag.Parse()
	if *outFile == "" || *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	log := cmd.NewLogger(cmd.SyslogConfig{StdoutLevel: 7})
	log.Info(cmd.VersionString())

	// Load configuration file.
	configData, err := os.ReadFile(*configFile)
	cmd.FailOnError(err, fmt.Sprintf("Reading %q", *configFile))

	// Unmarshal JSON config file.
	var cfg Config
	err = json.Unmarshal(configData, &cfg)
	cmd.FailOnError(err, "Unmarshaling config")

	features.Set(cfg.ContactExporter.Features)

	dbMap, err := sa.InitWrappedDb(cfg.ContactExporter.DB, nil, log)
	cmd.FailOnError(err, "While initializing dbMap")

	exporter := idExporter{
		log:   log,
		dbMap: dbMap,
		clk:   cmd.Clock(),
		grace: *grace,
	}

	var results idExporterResults
	if *hostnamesFile != "" {
		hostnames, err := unmarshalHostnames(*hostnamesFile)
		cmd.FailOnError(err, "Problem unmarshalling hostnames")

		results, err = exporter.findIDsForHostnames(context.TODO(), hostnames)
		cmd.FailOnError(err, "Could not find IDs for hostnames")

	} else if *withExampleHostnames {
		results, err = exporter.findIDsWithExampleHostnames(context.TODO())
		cmd.FailOnError(err, "Could not find IDs with hostnames")

	} else {
		results, err = exporter.findIDs(context.TODO())
		cmd.FailOnError(err, "Could not find IDs")
	}

	err = results.writeToFile(*outFile)
	cmd.FailOnError(err, fmt.Sprintf("Could not write result to outfile %q", *outFile))
}

func init() {
	cmd.RegisterCommand("id-exporter", main, &cmd.ConfigValidator{Config: &Config{}})
}
