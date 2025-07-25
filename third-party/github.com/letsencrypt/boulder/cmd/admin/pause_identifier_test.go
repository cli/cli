package main

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"testing"

	blog "github.com/letsencrypt/boulder/log"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	"google.golang.org/grpc"
)

func TestReadingPauseCSV(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		data            []string
		expectedRecords int
	}{
		{
			name: "No data in file",
			data: nil,
		},
		{
			name:            "valid",
			data:            []string{"1,dns,example.com"},
			expectedRecords: 1,
		},
		{
			name:            "valid with duplicates",
			data:            []string{"1,dns,example.com", "2,dns,example.org", "1,dns,example.com", "1,dns,example.net", "3,dns,example.gov", "3,dns,example.gov"},
			expectedRecords: 6,
		},
		{
			name: "invalid with multiple domains on the same line",
			data: []string{"1,dns,example.com,example.net"},
		},
		{
			name: "invalid just commas",
			data: []string{",,,"},
		},
		{
			name: "invalid only contains accountID",
			data: []string{"1"},
		},
		{
			name: "invalid only contains accountID and identifierType",
			data: []string{"1,dns"},
		},
		{
			name: "invalid missing identifierType",
			data: []string{"1,,example.com"},
		},
		{
			name: "invalid accountID isnt an int",
			data: []string{"blorple"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			log := blog.NewMock()
			a := admin{log: log}

			csvFile := path.Join(t.TempDir(), path.Base(t.Name()+".csv"))
			err := os.WriteFile(csvFile, []byte(strings.Join(testCase.data, "\n")), os.ModePerm)
			test.AssertNotError(t, err, "could not write temporary file")

			parsedData, err := a.readPausedAccountFile(csvFile)
			test.AssertNotError(t, err, "no error expected, but received one")
			test.AssertEquals(t, len(parsedData), testCase.expectedRecords)
		})
	}
}

// mockSAPaused is a mock which always succeeds. It records the PauseRequest it
// received, and returns the number of identifiers as a
// PauseIdentifiersResponse. It does not maintain state of repaused identifiers.
type mockSAPaused struct {
	sapb.StorageAuthorityClient
}

func (msa *mockSAPaused) PauseIdentifiers(ctx context.Context, in *sapb.PauseRequest, _ ...grpc.CallOption) (*sapb.PauseIdentifiersResponse, error) {
	return &sapb.PauseIdentifiersResponse{Paused: int64(len(in.Identifiers))}, nil
}

// mockSAPausedBroken is a mock which always errors.
type mockSAPausedBroken struct {
	sapb.StorageAuthorityClient
}

func (msa *mockSAPausedBroken) PauseIdentifiers(ctx context.Context, in *sapb.PauseRequest, _ ...grpc.CallOption) (*sapb.PauseIdentifiersResponse, error) {
	return nil, errors.New("its all jacked up")
}

func TestPauseIdentifiers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		data          []pauseCSVData
		saImpl        sapb.StorageAuthorityClient
		expectRespLen int
		expectErr     bool
	}{
		{
			name:      "no data",
			data:      nil,
			expectErr: true,
		},
		{
			name: "valid single entry",
			data: []pauseCSVData{
				{
					accountID:       1,
					identifierType:  "dns",
					identifierValue: "example.com",
				},
			},
			expectRespLen: 1,
		},
		{
			name:      "valid single entry but broken SA",
			expectErr: true,
			saImpl:    &mockSAPausedBroken{},
			data: []pauseCSVData{
				{
					accountID:       1,
					identifierType:  "dns",
					identifierValue: "example.com",
				},
			},
		},
		{
			name: "valid multiple entries with duplicates",
			data: []pauseCSVData{
				{
					accountID:       1,
					identifierType:  "dns",
					identifierValue: "example.com",
				},
				{
					accountID:       1,
					identifierType:  "dns",
					identifierValue: "example.com",
				},
				{
					accountID:       2,
					identifierType:  "dns",
					identifierValue: "example.org",
				},
				{
					accountID:       3,
					identifierType:  "dns",
					identifierValue: "example.net",
				},
				{
					accountID:       3,
					identifierType:  "dns",
					identifierValue: "example.org",
				},
			},
			expectRespLen: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			log := blog.NewMock()

			// Default to a working mock SA implementation
			if testCase.saImpl == nil {
				testCase.saImpl = &mockSAPaused{}
			}
			a := admin{sac: testCase.saImpl, log: log}

			responses, err := a.pauseIdentifiers(context.Background(), testCase.data, 10)
			if testCase.expectErr {
				test.AssertError(t, err, "should have errored, but did not")
			} else {
				test.AssertNotError(t, err, "should not have errored")
				// Batching will consolidate identifiers under the same account.
				test.AssertEquals(t, len(responses), testCase.expectRespLen)
			}
		})
	}
}
