package satest

import (
	"context"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateWorkingRegistration inserts a new, correct Registration into
// SA using GoodKey under the hood. This is used by various non-SA tests
// to initialize the a registration for the test to reference.
func CreateWorkingRegistration(t *testing.T, sa sapb.StorageAuthorityClient) *corepb.Registration {
	reg, err := sa.NewRegistration(context.Background(), &corepb.Registration{
		Key: []byte(`{
    "kty": "RSA",
    "n": "n4EPtAOCc9AlkeQHPzHStgAbgs7bTZLwUBZdR8_KuKPEHLd4rHVTeT-O-XV2jRojdNhxJWTDvNd7nqQ0VEiZQHz_AJmSCpMaJMRBSFKrKb2wqVwGU_NsYOYL-QtiWN2lbzcEe6XC0dApr5ydQLrHqkHHig3RBordaZ6Aj-oBHqFEHYpPe7Tpe-OfVfHd1E6cS6M1FZcD1NNLYD5lFHpPI9bTwJlsde3uhGqC0ZCuEHg8lhzwOHrtIQbS0FVbb9k3-tVTU4fg_3L_vniUFAKwuCLqKnS2BYwdq_mzSnbLY7h_qixoR7jig3__kRhuaxwUkRz5iaiQkqgc5gHdrNP5zw",
    "e": "AQAB"
}`),
		Contact:   []string{"mailto:foo@example.com"},
		CreatedAt: timestamppb.New(time.Date(2003, 5, 10, 0, 0, 0, 0, time.UTC)),
		Status:    string(core.StatusValid),
	})
	if err != nil {
		t.Fatalf("Unable to create new registration: %s", err)
	}
	return reg
}
