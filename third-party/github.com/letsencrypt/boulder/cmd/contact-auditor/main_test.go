package notmain

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/sa"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/test/vars"
)

var (
	regA *corepb.Registration
	regB *corepb.Registration
	regC *corepb.Registration
	regD *corepb.Registration
)

const (
	emailARaw = "test@example.com"
	emailBRaw = "example@notexample.com"
	emailCRaw = "test-example@notexample.com"
	telNum    = "666-666-7777"
)

func TestContactAuditor(t *testing.T) {
	testCtx := setup(t)
	defer testCtx.cleanUp()

	// Add some test registrations.
	testCtx.addRegistrations(t)

	resChan := make(chan *result, 10)
	err := testCtx.c.run(context.Background(), resChan)
	test.AssertNotError(t, err, "received error")

	// We should get back A, B, C, and D
	test.AssertEquals(t, len(resChan), 4)
	for entry := range resChan {
		err := validateContacts(entry.id, entry.createdAt, entry.contacts)
		switch entry.id {
		case regA.Id:
			// Contact validation policy sad path.
			test.AssertDeepEquals(t, entry.contacts, []string{"mailto:test@example.com"})
			test.AssertError(t, err, "failed to error on a contact that violates our e-mail policy")
		case regB.Id:
			// Ensure grace period was respected.
			test.AssertDeepEquals(t, entry.contacts, []string{"mailto:example@notexample.com"})
			test.AssertNotError(t, err, "received error for a valid contact entry")
		case regC.Id:
			// Contact validation happy path.
			test.AssertDeepEquals(t, entry.contacts, []string{"mailto:test-example@notexample.com"})
			test.AssertNotError(t, err, "received error for a valid contact entry")

			// Unmarshal Contact sad path.
			_, err := unmarshalContact([]byte("[ mailto:test@example.com ]"))
			test.AssertError(t, err, "failed to error while unmarshaling invalid Contact JSON")

			// Fix our JSON and ensure that the contact field returns
			// errors for our 2 additional contacts
			contacts, err := unmarshalContact([]byte(`[ "mailto:test@example.com", "tel:666-666-7777" ]`))
			test.AssertNotError(t, err, "received error while unmarshaling valid Contact JSON")

			// Ensure Contact validation now fails.
			err = validateContacts(entry.id, entry.createdAt, contacts)
			test.AssertError(t, err, "failed to error on 2 invalid Contact entries")
		case regD.Id:
			test.AssertDeepEquals(t, entry.contacts, []string{"tel:666-666-7777"})
			test.AssertError(t, err, "failed to error on an invalid contact entry")
		default:
			t.Errorf("ID: %d was not expected", entry.id)
		}
	}

	// Load results file.
	data, err := os.ReadFile(testCtx.c.resultsFile.Name())
	if err != nil {
		t.Error(err)
	}

	// Results file should contain 2 newlines, 1 for each result.
	contentLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	test.AssertEquals(t, len(contentLines), 2)

	// Each result entry should contain six tab separated columns.
	for _, line := range contentLines {
		test.AssertEquals(t, len(strings.Split(line, "\t")), 6)
	}
}

type testCtx struct {
	c       contactAuditor
	dbMap   *db.WrappedMap
	ssa     *sa.SQLStorageAuthority
	cleanUp func()
}

func (tc testCtx) addRegistrations(t *testing.T) {
	emailA := "mailto:" + emailARaw
	emailB := "mailto:" + emailBRaw
	emailC := "mailto:" + emailCRaw
	tel := "tel:" + telNum

	// Every registration needs a unique JOSE key
	jsonKeyA := []byte(`{
  "kty":"RSA",
  "n":"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw",
  "e":"AQAB"
}`)
	jsonKeyB := []byte(`{
  "kty":"RSA",
  "n":"z8bp-jPtHt4lKBqepeKF28g_QAEOuEsCIou6sZ9ndsQsEjxEOQxQ0xNOQezsKa63eogw8YS3vzjUcPP5BJuVzfPfGd5NVUdT-vSSwxk3wvk_jtNqhrpcoG0elRPQfMVsQWmxCAXCVRz3xbcFI8GTe-syynG3l-g1IzYIIZVNI6jdljCZML1HOMTTW4f7uJJ8mM-08oQCeHbr5ejK7O2yMSSYxW03zY-Tj1iVEebROeMv6IEEJNFSS4yM-hLpNAqVuQxFGetwtwjDMC1Drs1dTWrPuUAAjKGrP151z1_dE74M5evpAhZUmpKv1hY-x85DC6N0hFPgowsanmTNNiV75w",
  "e":"AAEAAQ"
}`)
	jsonKeyC := []byte(`{
  "kty":"RSA",
  "n":"rFH5kUBZrlPj73epjJjyCxzVzZuV--JjKgapoqm9pOuOt20BUTdHqVfC2oDclqM7HFhkkX9OSJMTHgZ7WaVqZv9u1X2yjdx9oVmMLuspX7EytW_ZKDZSzL-sCOFCuQAuYKkLbsdcA3eHBK_lwc4zwdeHFMKIulNvLqckkqYB9s8GpgNXBDIQ8GjR5HuJke_WUNjYHSd8jY1LU9swKWsLQe2YoQUz_ekQvBvBCoaFEtrtRaSJKNLIVDObXFr2TLIiFiM0Em90kK01-eQ7ZiruZTKomll64bRFPoNo4_uwubddg3xTqur2vdF3NyhTrYdvAgTem4uC0PFjEQ1bK_djBQ",
  "e":"AQAB"
}`)
	jsonKeyD := []byte(`{
  "kty":"RSA",
  "n":"rFH5kUBZrlPj73epjJjyCxzVzZuV--JjKgapoqm9pOuOt20BUTdHqVfC2oDclqM7HFhkkX9OSJMTHgZ7WaVqZv9u1X2yjdx9oVmMLuspX7EytW_ZKDZSzL-FCOFCuQAuYKkLbsdcA3eHBK_lwc4zwdeHFMKIulNvLqckkqYB9s8GpgNXBDIQ8GjR5HuJke_WUNjYHSd8jY1LU9swKWsLQe2YoQUz_ekQvBvBCoaFEtrtRaSJKNLIVDObXFr2TLIiFiM0Em90kK01-eQ7ZiruZTKomll64bRFPoNo4_uwubddg3xTqur2vdF3NyhTrYdvAgTem4uC0PFjEQ1bK_djBQ",
  "e":"AQAB"
}`)

	initialIP, err := net.ParseIP("127.0.0.1").MarshalText()
	test.AssertNotError(t, err, "Couldn't create initialIP")

	regA = &corepb.Registration{
		Id:        1,
		Contact:   []string{emailA},
		Key:       jsonKeyA,
		InitialIP: initialIP,
	}
	regB = &corepb.Registration{
		Id:        2,
		Contact:   []string{emailB},
		Key:       jsonKeyB,
		InitialIP: initialIP,
	}
	regC = &corepb.Registration{
		Id:        3,
		Contact:   []string{emailC},
		Key:       jsonKeyC,
		InitialIP: initialIP,
	}
	// Reg D has a `tel:` contact ACME URL
	regD = &corepb.Registration{
		Id:        4,
		Contact:   []string{tel},
		Key:       jsonKeyD,
		InitialIP: initialIP,
	}

	// Add the four test registrations
	ctx := context.Background()
	regA, err = tc.ssa.NewRegistration(ctx, regA)
	test.AssertNotError(t, err, "Couldn't store regA")
	regB, err = tc.ssa.NewRegistration(ctx, regB)
	test.AssertNotError(t, err, "Couldn't store regB")
	regC, err = tc.ssa.NewRegistration(ctx, regC)
	test.AssertNotError(t, err, "Couldn't store regC")
	regD, err = tc.ssa.NewRegistration(ctx, regD)
	test.AssertNotError(t, err, "Couldn't store regD")
}

func setup(t *testing.T) testCtx {
	log := blog.UseMock()

	// Using DBConnSAFullPerms to be able to insert registrations and
	// certificates
	dbMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	if err != nil {
		t.Fatalf("Couldn't connect to the database: %s", err)
	}

	// Make temp results file
	file, err := os.CreateTemp("", fmt.Sprintf("audit-%s", time.Now().Format("2006-01-02T15:04")))
	if err != nil {
		t.Fatal(err)
	}

	cleanUp := func() {
		test.ResetBoulderTestDatabase(t)
		file.Close()
		os.Remove(file.Name())
	}

	db, err := sa.DBMapForTest(vars.DBConnSAMailer)
	if err != nil {
		t.Fatalf("Couldn't connect to the database: %s", err)
	}

	ssa, err := sa.NewSQLStorageAuthority(dbMap, dbMap, nil, 1, 0, clock.New(), log, metrics.NoopRegisterer)
	if err != nil {
		t.Fatalf("unable to create SQLStorageAuthority: %s", err)
	}

	return testCtx{
		c: contactAuditor{
			db:          db,
			resultsFile: file,
			logger:      blog.NewMock(),
		},
		dbMap:   dbMap,
		ssa:     ssa,
		cleanUp: cleanUp,
	}
}
