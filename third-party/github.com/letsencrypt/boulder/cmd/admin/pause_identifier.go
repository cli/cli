package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/identifier"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// subcommandPauseIdentifier encapsulates the "admin pause-identifiers" command.
type subcommandPauseIdentifier struct {
	batchFile   string
	parallelism uint
}

var _ subcommand = (*subcommandPauseIdentifier)(nil)

func (p *subcommandPauseIdentifier) Desc() string {
	return "Administratively pause an account preventing it from attempting certificate issuance"
}

func (p *subcommandPauseIdentifier) Flags(flag *flag.FlagSet) {
	flag.StringVar(&p.batchFile, "batch-file", "", "Path to a CSV file containing (account ID, identifier type, identifier value)")
	flag.UintVar(&p.parallelism, "parallelism", 10, "The maximum number of concurrent pause requests to send to the SA (default: 10)")
}

func (p *subcommandPauseIdentifier) Run(ctx context.Context, a *admin) error {
	if p.batchFile == "" {
		return errors.New("the -batch-file flag is required")
	}

	idents, err := a.readPausedAccountFile(p.batchFile)
	if err != nil {
		return err
	}

	_, err = a.pauseIdentifiers(ctx, idents, p.parallelism)
	if err != nil {
		return err
	}

	return nil
}

// pauseIdentifiers concurrently pauses identifiers for each account using up to
// `parallelism` workers. It returns all pause responses and any accumulated
// errors.
func (a *admin) pauseIdentifiers(ctx context.Context, entries []pauseCSVData, parallelism uint) ([]*sapb.PauseIdentifiersResponse, error) {
	if len(entries) <= 0 {
		return nil, errors.New("cannot pause identifiers because no pauseData was sent")
	}

	accountToIdents := make(map[int64][]*corepb.Identifier)
	for _, entry := range entries {
		accountToIdents[entry.accountID] = append(accountToIdents[entry.accountID], &corepb.Identifier{
			Type:  string(entry.identifierType),
			Value: entry.identifierValue,
		})
	}

	var errCount atomic.Uint64
	respChan := make(chan *sapb.PauseIdentifiersResponse, len(accountToIdents))
	work := make(chan struct {
		accountID int64
		idents    []*corepb.Identifier
	}, parallelism)

	var wg sync.WaitGroup
	for i := uint(0); i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for data := range work {
				response, err := a.sac.PauseIdentifiers(ctx, &sapb.PauseRequest{
					RegistrationID: data.accountID,
					Identifiers:    data.idents,
				})
				if err != nil {
					errCount.Add(1)
					a.log.Errf("error pausing identifier(s) %q for account %d: %v", data.idents, data.accountID, err)
				} else {
					respChan <- response
				}
			}
		}()
	}

	for accountID, idents := range accountToIdents {
		work <- struct {
			accountID int64
			idents    []*corepb.Identifier
		}{accountID, idents}
	}
	close(work)
	wg.Wait()
	close(respChan)

	var responses []*sapb.PauseIdentifiersResponse
	for response := range respChan {
		responses = append(responses, response)
	}

	if errCount.Load() > 0 {
		return responses, fmt.Errorf("encountered %d errors while pausing identifiers; see logs above for details", errCount.Load())
	}

	return responses, nil
}

// pauseCSVData contains a golang representation of the data loaded in from a
// CSV file for pausing.
type pauseCSVData struct {
	accountID       int64
	identifierType  identifier.IdentifierType
	identifierValue string
}

// readPausedAccountFile parses the contents of a CSV into a slice of
// `pauseCSVData` objects and returns it or an error. It will skip malformed
// lines and continue processing until either the end of file marker is detected
// or other read error.
func (a *admin) readPausedAccountFile(filePath string) ([]pauseCSVData, error) {
	fp, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening paused account data file: %w", err)
	}
	defer fp.Close()

	reader := csv.NewReader(fp)

	// identifierValue can have 1 or more entries
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	var parsedRecords []pauseCSVData
	lineCounter := 0

	// Process contents of the CSV file
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}

		lineCounter++

		// We should have strictly 3 fields, note that just commas is considered
		// a valid CSV line.
		if len(record) != 3 {
			a.log.Infof("skipping: malformed line %d, should contain exactly 3 fields\n", lineCounter)
			continue
		}

		recordID := record[0]
		accountID, err := strconv.ParseInt(recordID, 10, 64)
		if err != nil || accountID == 0 {
			a.log.Infof("skipping: malformed accountID entry on line %d\n", lineCounter)
			continue
		}

		// Ensure that an identifier type is present, otherwise skip the line.
		if len(record[1]) == 0 {
			a.log.Infof("skipping: malformed identifierType entry on line %d\n", lineCounter)
			continue
		}

		if len(record[2]) == 0 {
			a.log.Infof("skipping: malformed identifierValue entry on line %d\n", lineCounter)
			continue
		}

		parsedRecord := pauseCSVData{
			accountID:       accountID,
			identifierType:  identifier.IdentifierType(record[1]),
			identifierValue: record[2],
		}
		parsedRecords = append(parsedRecords, parsedRecord)
	}
	a.log.Infof("detected %d valid record(s) from input file\n", len(parsedRecords))

	return parsedRecords, nil
}
