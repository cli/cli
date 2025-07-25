package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/unpause"
)

// subcommandUnpauseAccount encapsulates the "admin unpause-account" command.
type subcommandUnpauseAccount struct {
	accountID   int64
	batchFile   string
	parallelism uint
}

var _ subcommand = (*subcommandUnpauseAccount)(nil)

func (u *subcommandUnpauseAccount) Desc() string {
	return "Administratively unpause an account to allow certificate issuance attempts"
}

func (u *subcommandUnpauseAccount) Flags(flag *flag.FlagSet) {
	flag.Int64Var(&u.accountID, "account", 0, "A single account ID to unpause")
	flag.StringVar(&u.batchFile, "batch-file", "", "Path to a file containing multiple account IDs where each is separated by a newline")
	flag.UintVar(&u.parallelism, "parallelism", 10, "The maximum number of concurrent unpause requests to send to the SA (default: 10)")
}

func (u *subcommandUnpauseAccount) Run(ctx context.Context, a *admin) error {
	// This is a map of all input-selection flags to whether or not they were set
	// to a non-default value. We use this to ensure that exactly one input
	// selection flag was given on the command line.
	setInputs := map[string]bool{
		"-account":    u.accountID != 0,
		"-batch-file": u.batchFile != "",
	}
	activeFlag, err := findActiveInputMethodFlag(setInputs)
	if err != nil {
		return err
	}

	var regIDs []int64
	switch activeFlag {
	case "-account":
		regIDs = []int64{u.accountID}
	case "-batch-file":
		regIDs, err = a.readUnpauseAccountFile(u.batchFile)
	default:
		return errors.New("no recognized input method flag set (this shouldn't happen)")
	}
	if err != nil {
		return fmt.Errorf("collecting serials to revoke: %w", err)
	}

	_, err = a.unpauseAccounts(ctx, regIDs, u.parallelism)
	if err != nil {
		return err
	}

	return nil
}

type unpauseCount struct {
	accountID int64
	count     int64
}

// unpauseAccount concurrently unpauses all identifiers for each account using
// up to `parallelism` workers. It returns a count of the number of identifiers
// unpaused for each account and any accumulated errors.
func (a *admin) unpauseAccounts(ctx context.Context, accountIDs []int64, parallelism uint) ([]unpauseCount, error) {
	if len(accountIDs) <= 0 {
		return nil, errors.New("no account IDs provided for unpausing")
	}
	slices.Sort(accountIDs)
	accountIDs = slices.Compact(accountIDs)

	countChan := make(chan unpauseCount, len(accountIDs))
	work := make(chan int64)

	var wg sync.WaitGroup
	var errCount atomic.Uint64
	for i := uint(0); i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for accountID := range work {
				totalCount := int64(0)
				for {
					response, err := a.sac.UnpauseAccount(ctx, &sapb.RegistrationID{Id: accountID})
					if err != nil {
						errCount.Add(1)
						a.log.Errf("error unpausing accountID %d: %v", accountID, err)
						break
					}
					totalCount += response.Count
					if response.Count < unpause.RequestLimit {
						// All identifiers have been unpaused.
						break
					}
				}
				countChan <- unpauseCount{accountID: accountID, count: totalCount}
			}
		}()
	}

	go func() {
		for _, accountID := range accountIDs {
			work <- accountID
		}
		close(work)
	}()

	go func() {
		wg.Wait()
		close(countChan)
	}()

	var unpauseCounts []unpauseCount
	for count := range countChan {
		unpauseCounts = append(unpauseCounts, count)
	}

	if errCount.Load() > 0 {
		return unpauseCounts, fmt.Errorf("encountered %d errors while unpausing; see logs above for details", errCount.Load())
	}

	return unpauseCounts, nil
}

// readUnpauseAccountFile parses the contents of a file containing one account
// ID per into a slice of int64s. It will skip malformed records and continue
// processing until the end of file marker.
func (a *admin) readUnpauseAccountFile(filePath string) ([]int64, error) {
	fp, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening paused account data file: %w", err)
	}
	defer fp.Close()

	var unpauseAccounts []int64
	lineCounter := 0
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		lineCounter++
		regID, err := strconv.ParseInt(scanner.Text(), 10, 64)
		if err != nil {
			a.log.Infof("skipping: malformed account ID entry on line %d\n", lineCounter)
			continue
		}
		unpauseAccounts = append(unpauseAccounts, regID)
	}

	if err := scanner.Err(); err != nil {
		return nil, scanner.Err()
	}

	return unpauseAccounts, nil
}
