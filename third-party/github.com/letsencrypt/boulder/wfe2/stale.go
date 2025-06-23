package wfe2

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/web"
)

// requiredStale checks if a request is a GET request with a logEvent indicating
// the endpoint starts with getAPIPrefix. If true then the caller is expected to
// apply staleness requirements via staleEnoughToGETOrder, staleEnoughToGETCert
// and staleEnoughToGETAuthz.
func requiredStale(req *http.Request, logEvent *web.RequestEvent) bool {
	return req.Method == http.MethodGet && strings.HasPrefix(logEvent.Endpoint, getAPIPrefix)
}

// staleEnoughToGETOrder checks if the given order was created long enough ago
// in the past to be acceptably stale for accessing via the Boulder specific GET
// API.
func (wfe *WebFrontEndImpl) staleEnoughToGETOrder(order *corepb.Order) *probs.ProblemDetails {
	return wfe.staleEnoughToGET("Order", order.Created.AsTime())
}

// staleEnoughToGETCert checks if the given cert was issued long enough in the
// past to be acceptably stale for accessing via the Boulder specific GET API.
func (wfe *WebFrontEndImpl) staleEnoughToGETCert(cert *corepb.Certificate) *probs.ProblemDetails {
	return wfe.staleEnoughToGET("Certificate", cert.Issued.AsTime())
}

// staleEnoughToGETAuthz checks if the given authorization was created long
// enough ago in the past to be acceptably stale for accessing via the Boulder
// specific GET API. Since authorization creation date is not tracked directly
// the appropriate lifetime for the authz is subtracted from the expiry to find
// the creation date.
func (wfe *WebFrontEndImpl) staleEnoughToGETAuthz(authzPB *corepb.Authorization) *probs.ProblemDetails {
	// If the authorization was deactivated we cannot reliably tell what the creation date was
	// because we can't easily tell if it was pending or finalized before deactivation.
	// As these authorizations can no longer be used for anything, just make them immediately
	// available for access.
	if core.AcmeStatus(authzPB.Status) == core.StatusDeactivated {
		return nil
	}
	// We don't directly track authorization creation time. Instead subtract the
	// pendingAuthorization lifetime from the expiry. This will be inaccurate if
	// we change the pendingAuthorizationLifetime but is sufficient for the weak
	// staleness requirements of the GET API.
	createdTime := authzPB.Expires.AsTime().Add(-wfe.pendingAuthorizationLifetime)
	// if the authz is valid then we need to subtract the authorizationLifetime
	// instead of the pendingAuthorizationLifetime.
	if core.AcmeStatus(authzPB.Status) == core.StatusValid {
		createdTime = authzPB.Expires.AsTime().Add(-wfe.authorizationLifetime)
	}
	return wfe.staleEnoughToGET("Authorization", createdTime)
}

// staleEnoughToGET checks that the createDate for the given resource is at
// least wfe.staleTimeout in the past. If the resource is newer than the
// wfe.staleTimeout then an unauthorized problem is returned.
func (wfe *WebFrontEndImpl) staleEnoughToGET(resourceType string, createDate time.Time) *probs.ProblemDetails {
	if wfe.clk.Since(createDate) < wfe.staleTimeout {
		return probs.Unauthorized(fmt.Sprintf(
			"%s is too new for GET API. "+
				"You should only use this non-standard API to access resources created more than %s ago",
			resourceType,
			wfe.staleTimeout))
	}
	return nil
}
