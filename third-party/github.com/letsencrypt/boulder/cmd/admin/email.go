package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/letsencrypt/boulder/sa"
)

// subcommandUpdateEmail encapsulates the "admin update-email" command.
//
// Note that this command may be very slow, as the initial query to find the set
// of accounts which have a matching contact email address does not use a
// database index. Therefore, when updating the found accounts, it does not exit
// on failure, preferring to continue and make as much progress as possible.
type subcommandUpdateEmail struct {
	address string
	clear   bool
}

var _ subcommand = (*subcommandUpdateEmail)(nil)

func (s *subcommandUpdateEmail) Desc() string {
	return "Change or remove an email address across all accounts"
}

func (s *subcommandUpdateEmail) Flags(flag *flag.FlagSet) {
	flag.StringVar(&s.address, "address", "", "Email address to update")
	flag.BoolVar(&s.clear, "clear", false, "If set, remove the address")
}

func (s *subcommandUpdateEmail) Run(ctx context.Context, a *admin) error {
	if s.address == "" {
		return errors.New("the -address flag is required")
	}

	if s.clear {
		return a.clearEmail(ctx, s.address)
	}

	return errors.New("no action to perform on the given email was specified")
}

func (a *admin) clearEmail(ctx context.Context, address string) error {
	a.log.AuditInfof("Scanning database for accounts with email addresses matching %q in order to clear the email addresses.", address)

	// We use SQL `CONCAT` rather than interpolating with `+` or `%s` because we want to
	// use a `?` placeholder for the email, which prevents SQL injection.
	// Since this uses a substring match, it is important
	// to subsequently parse the JSON list of addresses and look for exact matches.
	// Because this does not use an index, it is very slow.
	var regIDs []int64
	_, err := a.dbMap.Select(ctx, &regIDs, "SELECT id FROM registrations WHERE contact LIKE CONCAT('%\"mailto:', ?, '\"%')", address)
	if err != nil {
		return fmt.Errorf("identifying matching accounts: %w", err)
	}

	a.log.Infof("Found %d registration IDs matching email %q.", len(regIDs), address)

	failures := 0
	for _, regID := range regIDs {
		if a.dryRun {
			a.log.Infof("dry-run: remove %q from account %d", address, regID)
			continue
		}

		err := sa.ClearEmail(ctx, a.dbMap, regID, address)
		if err != nil {
			// Log, but don't fail, because it took a long time to find the relevant registration IDs
			// and we don't want to have to redo that work.
			a.log.AuditErrf("failed to clear email %q for registration ID %d: %s", address, regID, err)
			failures++
		} else {
			a.log.AuditInfof("cleared email %q for registration ID %d", address, regID)
		}
	}
	if failures > 0 {
		return fmt.Errorf("failed to clear email for %d out of %d registration IDs", failures, len(regIDs))
	}

	return nil
}
