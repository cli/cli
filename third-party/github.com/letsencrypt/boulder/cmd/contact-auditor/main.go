package notmain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/sa"
)

type contactAuditor struct {
	db            *db.WrappedMap
	resultsFile   *os.File
	writeToStdout bool
	logger        blog.Logger
}

type result struct {
	id        int64
	contacts  []string
	createdAt string
}

func unmarshalContact(contact []byte) ([]string, error) {
	var contacts []string
	err := json.Unmarshal(contact, &contacts)
	if err != nil {
		return nil, err
	}
	return contacts, nil
}

func validateContacts(id int64, createdAt string, contacts []string) error {
	// Setup a buffer to store any validation problems we encounter.
	var probsBuff strings.Builder

	// Helper to write validation problems to our buffer.
	writeProb := func(contact string, prob string) {
		// Add validation problem to buffer.
		fmt.Fprintf(&probsBuff, "%d\t%s\tvalidation\t%q\t%q\t%q\n", id, createdAt, contact, prob, contacts)
	}

	for _, contact := range contacts {
		if strings.HasPrefix(contact, "mailto:") {
			err := policy.ValidEmail(strings.TrimPrefix(contact, "mailto:"))
			if err != nil {
				writeProb(contact, err.Error())
			}
		} else {
			writeProb(contact, "missing 'mailto:' prefix")
		}
	}

	if probsBuff.Len() != 0 {
		return errors.New(probsBuff.String())
	}
	return nil
}

// beginAuditQuery executes the audit query and returns a cursor used to
// stream the results.
func (c contactAuditor) beginAuditQuery(ctx context.Context) (*sql.Rows, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT DISTINCT id, contact, createdAt
		FROM registrations
		WHERE contact NOT IN ('[]', 'null');`)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (c contactAuditor) writeResults(result string) {
	if c.writeToStdout {
		_, err := fmt.Print(result)
		if err != nil {
			c.logger.Errf("Error while writing result to stdout: %s", err)
		}
	}

	if c.resultsFile != nil {
		_, err := c.resultsFile.WriteString(result)
		if err != nil {
			c.logger.Errf("Error while writing result to file: %s", err)
		}
	}
}

// run retrieves a cursor from `beginAuditQuery` and then audits the
// `contact` column of all returned rows for abnormalities or policy
// violations.
func (c contactAuditor) run(ctx context.Context, resChan chan *result) error {
	c.logger.Infof("Beginning database query")
	rows, err := c.beginAuditQuery(ctx)
	if err != nil {
		return err
	}

	for rows.Next() {
		var id int64
		var contact []byte
		var createdAt string
		err := rows.Scan(&id, &contact, &createdAt)
		if err != nil {
			return err
		}

		contacts, err := unmarshalContact(contact)
		if err != nil {
			c.writeResults(fmt.Sprintf("%d\t%s\tunmarshal\t%q\t%q\n", id, createdAt, contact, err))
		}

		err = validateContacts(id, createdAt, contacts)
		if err != nil {
			c.writeResults(err.Error())
		}

		// Only used for testing.
		if resChan != nil {
			resChan <- &result{id, contacts, createdAt}
		}
	}
	// Ensure the query wasn't interrupted before it could complete.
	err = rows.Close()
	if err != nil {
		return err
	} else {
		c.logger.Info("Query completed successfully")
	}

	// Only used for testing.
	if resChan != nil {
		close(resChan)
	}

	return nil
}

type Config struct {
	ContactAuditor struct {
		DB cmd.DBConfig
	}
}

func main() {
	configFile := flag.String("config", "", "File containing a JSON config.")
	writeToStdout := flag.Bool("to-stdout", false, "Print the audit results to stdout.")
	writeToFile := flag.Bool("to-file", false, "Write the audit results to a file.")
	flag.Parse()

	logger := cmd.NewLogger(cmd.SyslogConfig{StdoutLevel: 7})
	logger.Info(cmd.VersionString())

	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Load config from JSON.
	configData, err := os.ReadFile(*configFile)
	cmd.FailOnError(err, fmt.Sprintf("Error reading config file: %q", *configFile))

	var cfg Config
	err = json.Unmarshal(configData, &cfg)
	cmd.FailOnError(err, "Couldn't unmarshal config")

	db, err := sa.InitWrappedDb(cfg.ContactAuditor.DB, nil, logger)
	cmd.FailOnError(err, "Couldn't setup database client")

	var resultsFile *os.File
	if *writeToFile {
		resultsFile, err = os.Create(
			fmt.Sprintf("contact-audit-%s.tsv", time.Now().Format("2006-01-02T15:04")),
		)
		cmd.FailOnError(err, "Failed to create results file")
	}

	// Setup and run contact-auditor.
	auditor := contactAuditor{
		db:            db,
		resultsFile:   resultsFile,
		writeToStdout: *writeToStdout,
		logger:        logger,
	}

	logger.Info("Running contact-auditor")

	err = auditor.run(context.TODO(), nil)
	cmd.FailOnError(err, "Audit was interrupted, results may be incomplete")

	logger.Info("Audit finished successfully")

	if *writeToFile {
		logger.Infof("Audit results were written to: %s", resultsFile.Name())
		resultsFile.Close()
	}

}

func init() {
	cmd.RegisterCommand("contact-auditor", main, &cmd.ConfigValidator{Config: &Config{}})
}
