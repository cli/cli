package loglist

import (
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/test"
)

func TestNew(t *testing.T) {

}

func TestSubset(t *testing.T) {
	input := List{
		Log{Name: "Log A1"},
		Log{Name: "Log A2"},
		Log{Name: "Log B1"},
		Log{Name: "Log B2"},
		Log{Name: "Log C1"},
		Log{Name: "Log C2"},
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
		Log{Name: "Log B1"},
		Log{Name: "Log A1"},
		Log{Name: "Log A2"},
	}
	actual, err = input.subset([]string{"Log B1", "Log A1", "Log A2"})
	test.AssertNotError(t, err, "normal usage should not error")
	test.AssertDeepEquals(t, actual, expected)
}

func TestForPurpose(t *testing.T) {
	input := List{
		Log{Name: "Log A1", Operator: "A", State: loglist3.UsableLogStatus},
		Log{Name: "Log A2", Operator: "A", State: loglist3.RejectedLogStatus},
		Log{Name: "Log B1", Operator: "B", State: loglist3.UsableLogStatus},
		Log{Name: "Log B2", Operator: "B", State: loglist3.RetiredLogStatus},
		Log{Name: "Log C1", Operator: "C", State: loglist3.PendingLogStatus},
		Log{Name: "Log C2", Operator: "C", State: loglist3.ReadOnlyLogStatus},
	}
	expected := List{
		Log{Name: "Log A1", Operator: "A", State: loglist3.UsableLogStatus},
		Log{Name: "Log B1", Operator: "B", State: loglist3.UsableLogStatus},
	}
	actual, err := input.forPurpose(Issuance)
	test.AssertNotError(t, err, "should have two acceptable logs")
	test.AssertDeepEquals(t, actual, expected)

	input = List{
		Log{Name: "Log A1", Operator: "A", State: loglist3.UsableLogStatus},
		Log{Name: "Log A2", Operator: "A", State: loglist3.RejectedLogStatus},
		Log{Name: "Log B1", Operator: "B", State: loglist3.QualifiedLogStatus},
		Log{Name: "Log B2", Operator: "B", State: loglist3.RetiredLogStatus},
		Log{Name: "Log C1", Operator: "C", State: loglist3.PendingLogStatus},
		Log{Name: "Log C2", Operator: "C", State: loglist3.ReadOnlyLogStatus},
	}
	_, err = input.forPurpose(Issuance)
	test.AssertError(t, err, "should only have one acceptable log")

	expected = List{
		Log{Name: "Log A1", Operator: "A", State: loglist3.UsableLogStatus},
		Log{Name: "Log C2", Operator: "C", State: loglist3.ReadOnlyLogStatus},
	}
	actual, err = input.forPurpose(Validation)
	test.AssertNotError(t, err, "should have two acceptable logs")
	test.AssertDeepEquals(t, actual, expected)

	expected = List{
		Log{Name: "Log A1", Operator: "A", State: loglist3.UsableLogStatus},
		Log{Name: "Log B1", Operator: "B", State: loglist3.QualifiedLogStatus},
		Log{Name: "Log C1", Operator: "C", State: loglist3.PendingLogStatus},
	}
	actual, err = input.forPurpose(Informational)
	test.AssertNotError(t, err, "should have three acceptable logs")
	test.AssertDeepEquals(t, actual, expected)
}

func TestForTime(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	input := List{
		Log{Name: "Fully Bound", StartInclusive: fc.Now().Add(-time.Hour), EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Open End", StartInclusive: fc.Now().Add(-time.Hour)},
		Log{Name: "Open Start", EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Fully Open"},
	}

	expected := List{
		Log{Name: "Fully Bound", StartInclusive: fc.Now().Add(-time.Hour), EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Open End", StartInclusive: fc.Now().Add(-time.Hour)},
		Log{Name: "Open Start", EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Fully Open"},
	}
	actual := input.ForTime(fc.Now())
	test.AssertDeepEquals(t, actual, expected)

	expected = List{
		Log{Name: "Fully Bound", StartInclusive: fc.Now().Add(-time.Hour), EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Open End", StartInclusive: fc.Now().Add(-time.Hour)},
		Log{Name: "Open Start", EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Fully Open"},
	}
	actual = input.ForTime(fc.Now().Add(-time.Hour))
	test.AssertDeepEquals(t, actual, expected)

	expected = List{
		Log{Name: "Open Start", EndExclusive: fc.Now().Add(time.Hour)},
		Log{Name: "Fully Open"},
	}
	actual = input.ForTime(fc.Now().Add(-2 * time.Hour))
	test.AssertDeepEquals(t, actual, expected)

	expected = List{
		Log{Name: "Open End", StartInclusive: fc.Now().Add(-time.Hour)},
		Log{Name: "Fully Open"},
	}
	actual = input.ForTime(fc.Now().Add(time.Hour))
	test.AssertDeepEquals(t, actual, expected)
}

func TestPermute(t *testing.T) {
	input := List{
		Log{Name: "Log A1"},
		Log{Name: "Log A2"},
		Log{Name: "Log B1"},
		Log{Name: "Log B2"},
		Log{Name: "Log C1"},
		Log{Name: "Log C2"},
	}

	foundIndices := make(map[string]map[int]int)
	for _, log := range input {
		foundIndices[log.Name] = make(map[int]int)
	}

	for range 100 {
		actual := input.Permute()
		for index, log := range actual {
			foundIndices[log.Name][index]++
		}
	}

	for name, counts := range foundIndices {
		for index, count := range counts {
			if count == 0 {
				t.Errorf("Log %s appeared at index %d too few times", name, index)
			}
		}
	}
}

func TestGetByID(t *testing.T) {
	input := List{
		Log{Name: "Log A1", Id: "ID A1"},
		Log{Name: "Log B1", Id: "ID B1"},
	}

	expected := Log{Name: "Log A1", Id: "ID A1"}
	actual, err := input.GetByID("ID A1")
	test.AssertNotError(t, err, "should have found log")
	test.AssertDeepEquals(t, actual, expected)

	_, err = input.GetByID("Other ID")
	test.AssertError(t, err, "should not have found log")
}
