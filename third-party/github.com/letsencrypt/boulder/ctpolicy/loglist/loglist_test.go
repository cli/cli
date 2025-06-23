package loglist

import (
	"testing"
	"time"

	"github.com/letsencrypt/boulder/test"
)

func TestNew(t *testing.T) {

}

func TestSubset(t *testing.T) {
	input := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1"},
			"ID A2": Log{Name: "Log A2"},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1"},
			"ID B2": Log{Name: "Log B2"},
		},
		"Operator C": {
			"ID C1": Log{Name: "Log C1"},
			"ID C2": Log{Name: "Log C2"},
		},
	}

	actual, err := input.subset(nil)
	test.AssertNotError(t, err, "nil names should not error")
	test.AssertEquals(t, len(actual), 0)

	actual, err = input.subset([]string{})
	test.AssertNotError(t, err, "empty names should not error")
	test.AssertEquals(t, len(actual), 0)

	actual, err = input.subset([]string{"Other Log"})
	test.AssertError(t, err, "wrong name should result in error")
	test.AssertEquals(t, len(actual), 0)

	expected := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1"},
			"ID A2": Log{Name: "Log A2"},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1"},
		},
	}
	actual, err = input.subset([]string{"Log B1", "Log A1", "Log A2"})
	test.AssertNotError(t, err, "normal usage should not error")
	test.AssertDeepEquals(t, actual, expected)
}

func TestForPurpose(t *testing.T) {
	input := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
			"ID A2": Log{Name: "Log A2", State: rejected},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: usable},
			"ID B2": Log{Name: "Log B2", State: retired},
		},
		"Operator C": {
			"ID C1": Log{Name: "Log C1", State: pending},
			"ID C2": Log{Name: "Log C2", State: readonly},
		},
	}
	expected := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: usable},
		},
	}
	actual, err := input.forPurpose(Issuance)
	test.AssertNotError(t, err, "should have two acceptable logs")
	test.AssertDeepEquals(t, actual, expected)

	input = List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
			"ID A2": Log{Name: "Log A2", State: rejected},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: qualified},
			"ID B2": Log{Name: "Log B2", State: retired},
		},
		"Operator C": {
			"ID C1": Log{Name: "Log C1", State: pending},
			"ID C2": Log{Name: "Log C2", State: readonly},
		},
	}
	_, err = input.forPurpose(Issuance)
	test.AssertError(t, err, "should only have one acceptable log")

	expected = List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
		},
		"Operator C": {
			"ID C2": Log{Name: "Log C2", State: readonly},
		},
	}
	actual, err = input.forPurpose(Validation)
	test.AssertNotError(t, err, "should have two acceptable logs")
	test.AssertDeepEquals(t, actual, expected)

	expected = List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: qualified},
		},
		"Operator C": {
			"ID C1": Log{Name: "Log C1", State: pending},
		},
	}
	actual, err = input.forPurpose(Informational)
	test.AssertNotError(t, err, "should have three acceptable logs")
	test.AssertDeepEquals(t, actual, expected)
}

func TestOperatorForLogID(t *testing.T) {
	input := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: qualified},
		},
	}

	actual, err := input.OperatorForLogID("ID B1")
	test.AssertNotError(t, err, "should have found log")
	test.AssertEquals(t, actual, "Operator B")

	_, err = input.OperatorForLogID("Other ID")
	test.AssertError(t, err, "should not have found log")
}

func TestPermute(t *testing.T) {
	input := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", State: usable},
			"ID A2": Log{Name: "Log A2", State: rejected},
		},
		"Operator B": {
			"ID B1": Log{Name: "Log B1", State: qualified},
			"ID B2": Log{Name: "Log B2", State: retired},
		},
		"Operator C": {
			"ID C1": Log{Name: "Log C1", State: pending},
			"ID C2": Log{Name: "Log C2", State: readonly},
		},
	}

	actual := input.Permute()
	test.AssertEquals(t, len(actual), 3)
	test.AssertSliceContains(t, actual, "Operator A")
	test.AssertSliceContains(t, actual, "Operator B")
	test.AssertSliceContains(t, actual, "Operator C")
}

func TestPickOne(t *testing.T) {
	date0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	date1 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

	input := List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1"},
		},
	}
	_, _, err := input.PickOne("Operator B", date0)
	test.AssertError(t, err, "should have failed to find operator")

	input = List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", StartInclusive: date0, EndExclusive: date1},
		},
	}
	_, _, err = input.PickOne("Operator A", date2)
	test.AssertError(t, err, "should have failed to find log")
	_, _, err = input.PickOne("Operator A", date1)
	test.AssertError(t, err, "should have failed to find log")
	_, _, err = input.PickOne("Operator A", date0)
	test.AssertNotError(t, err, "should have found a log")
	_, _, err = input.PickOne("Operator A", date0.Add(time.Hour))
	test.AssertNotError(t, err, "should have found a log")

	input = List{
		"Operator A": {
			"ID A1": Log{Name: "Log A1", StartInclusive: date0, EndExclusive: date1, Key: "KA1", Url: "UA1"},
			"ID A2": Log{Name: "Log A2", StartInclusive: date1, EndExclusive: date2, Key: "KA2", Url: "UA2"},
			"ID B1": Log{Name: "Log B1", StartInclusive: date0, EndExclusive: date1, Key: "KB1", Url: "UB1"},
			"ID B2": Log{Name: "Log B2", StartInclusive: date1, EndExclusive: date2, Key: "KB2", Url: "UB2"},
		},
	}
	url, key, err := input.PickOne("Operator A", date0.Add(time.Hour))
	test.AssertNotError(t, err, "should have found a log")
	test.AssertSliceContains(t, []string{"UA1", "UB1"}, url)
	test.AssertSliceContains(t, []string{"KA1", "KB1"}, key)
}
