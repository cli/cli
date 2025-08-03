package validator

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestLineValidAccepts(t *testing.T) {
	err := lineValid("2020-07-06T18:07:43.109389+00:00 70877f679c72 datacenter 6 boulder-wfe[1595]: kKG6cwA Caught SIGTERM")
	test.AssertNotError(t, err, "errored on valid checksum")
}

func TestLineValidRejects(t *testing.T) {
	err := lineValid("2020-07-06T18:07:43.109389+00:00 70877f679c72 datacenter 6 boulder-wfe[1595]: xxxxxxx Caught SIGTERM")
	test.AssertError(t, err, "didn't error on invalid checksum")
}

func TestLineValidRejectsNotAChecksum(t *testing.T) {
	err := lineValid("2020-07-06T18:07:43.109389+00:00 70877f679c72 datacenter 6 boulder-wfe[1595]: xxxx Caught SIGTERM")
	test.AssertError(t, err, "didn't error on invalid checksum")
	test.AssertErrorIs(t, err, errInvalidChecksum)
}

func TestLineValidNonOurobouros(t *testing.T) {
	err := lineValid("2020-07-06T18:07:43.109389+00:00 70877f679c72 datacenter 6 boulder-wfe[1595]: xxxxxxx Caught SIGTERM")
	test.AssertError(t, err, "didn't error on invalid checksum")

	selfOutput := "2020-07-06T18:07:43.109389+00:00 70877f679c72 datacenter 6 log-validator[1337]: xxxxxxx " + err.Error()
	err2 := lineValid(selfOutput)
	test.AssertNotError(t, err2, "expected no error when feeding lineValid's error output into itself")
}
