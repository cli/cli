//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/eggsampler/acme/v3"
	_ "github.com/go-sql-driver/mysql"

	"github.com/letsencrypt/boulder/test"
)

func TestAdminClearEmail(t *testing.T) {
	t.Parallel()
	os.Setenv("DIRECTORY", "http://boulder.service.consul:4001/directory")

	// Note that `example@mail.example.letsencrypt.org` is a substring of `long-example@mail.example.letsencrypt.org`.
	// We specifically want to test that the superstring does not get removed, even though we use substring matching
	// as an initial filter.
	client1, err := makeClient("mailto:example@mail.example.letsencrypt.org", "mailto:long-example@mail.example.letsencrypt.org", "mailto:third-example@mail.example.letsencrypt.org")
	test.AssertNotError(t, err, "creating first acme client")

	client2, err := makeClient("mailto:example@mail.example.letsencrypt.org")
	test.AssertNotError(t, err, "creating second acme client")

	client3, err := makeClient("mailto:other@mail.example.letsencrypt.org")
	test.AssertNotError(t, err, "creating second acme client")

	deleteMe := "example@mail.example.letsencrypt.org"
	config := fmt.Sprintf("%s/%s", os.Getenv("BOULDER_CONFIG_DIR"), "admin.json")
	cmd := exec.Command(
		"./bin/admin",
		"-config", config,
		"-dry-run=false",
		"update-email",
		"-address", deleteMe,
		"-clear")
	output, err := cmd.CombinedOutput()
	test.AssertNotError(t, err, fmt.Sprintf("clearing email via admin tool (%s): %s", cmd, string(output)))
	t.Logf("clear-email output: %s\n", string(output))

	updatedAccount1, err := client1.NewAccountOptions(client1.PrivateKey, acme.NewAcctOptOnlyReturnExisting())
	test.AssertNotError(t, err, "fetching updated account for first client")

	t.Log(updatedAccount1.Contact)
	test.AssertDeepEquals(t, updatedAccount1.Contact,
		[]string{"mailto:long-example@mail.example.letsencrypt.org", "mailto:third-example@mail.example.letsencrypt.org"})

	updatedAccount2, err := client2.NewAccountOptions(client2.PrivateKey, acme.NewAcctOptOnlyReturnExisting())
	test.AssertNotError(t, err, "fetching updated account for second client")
	test.AssertDeepEquals(t, updatedAccount2.Contact, []string(nil))

	updatedAccount3, err := client3.NewAccountOptions(client3.PrivateKey, acme.NewAcctOptOnlyReturnExisting())
	test.AssertNotError(t, err, "fetching updated account for third client")
	test.AssertDeepEquals(t, updatedAccount3.Contact, []string{"mailto:other@mail.example.letsencrypt.org"})
}
