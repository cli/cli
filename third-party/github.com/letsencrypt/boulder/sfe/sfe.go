package sfe

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics/measured_http"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/unpause"
)

const (
	unpausePostForm = unpause.APIPrefix + "/do-unpause"
	unpauseStatus   = unpause.APIPrefix + "/unpause-status"
)

var (
	//go:embed all:static
	staticFS embed.FS

	//go:embed all:templates all:pages all:static
	dynamicFS embed.FS
)

// SelfServiceFrontEndImpl provides all the logic for Boulder's selfservice
// frontend web-facing interface, i.e., a portal where a subscriber can unpause
// their account. Its methods are primarily handlers for HTTPS requests for the
// various non-ACME functions.
type SelfServiceFrontEndImpl struct {
	ra rapb.RegistrationAuthorityClient
	sa sapb.StorageAuthorityReadOnlyClient

	log blog.Logger
	clk clock.Clock

	// requestTimeout is the per-request overall timeout.
	requestTimeout time.Duration

	unpauseHMACKey []byte
	templatePages  *template.Template
}

// NewSelfServiceFrontEndImpl constructs a web service for Boulder
func NewSelfServiceFrontEndImpl(
	stats prometheus.Registerer,
	clk clock.Clock,
	logger blog.Logger,
	requestTimeout time.Duration,
	rac rapb.RegistrationAuthorityClient,
	sac sapb.StorageAuthorityReadOnlyClient,
	unpauseHMACKey []byte,
) (SelfServiceFrontEndImpl, error) {

	// Parse the files once at startup to avoid each request causing the server
	// to JIT parse. The pages are stored in an in-memory embed.FS to prevent
	// unnecessary filesystem I/O on a physical HDD.
	tmplPages := template.Must(template.New("pages").ParseFS(dynamicFS, "templates/layout.html", "pages/*"))

	sfe := SelfServiceFrontEndImpl{
		log:            logger,
		clk:            clk,
		requestTimeout: requestTimeout,
		ra:             rac,
		sa:             sac,
		unpauseHMACKey: unpauseHMACKey,
		templatePages:  tmplPages,
	}

	return sfe, nil
}

// handleWithTimeout registers a handler with a timeout using an
// http.TimeoutHandler.
func (sfe *SelfServiceFrontEndImpl) handleWithTimeout(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	timeout := sfe.requestTimeout
	if timeout <= 0 {
		// Default to 5 minutes if no timeout is set.
		timeout = 5 * time.Minute
	}
	timeoutHandler := http.TimeoutHandler(handler, timeout, "Request timed out")
	mux.Handle(path, timeoutHandler)
}

// Handler returns an http.Handler that uses various functions for various
// non-ACME-specified paths. Each endpoint should have a corresponding HTML
// page that shares the same name as the endpoint.
func (sfe *SelfServiceFrontEndImpl) Handler(stats prometheus.Registerer, oTelHTTPOptions ...otelhttp.Option) http.Handler {
	mux := http.NewServeMux()

	sfs, _ := fs.Sub(staticFS, "static")
	staticAssetsHandler := http.StripPrefix("/static/", http.FileServerFS(sfs))
	mux.Handle("GET /static/", staticAssetsHandler)

	sfe.handleWithTimeout(mux, "/", sfe.Index)
	sfe.handleWithTimeout(mux, "GET /build", sfe.BuildID)
	sfe.handleWithTimeout(mux, "GET "+unpause.GetForm, sfe.UnpauseForm)
	sfe.handleWithTimeout(mux, "POST "+unpausePostForm, sfe.UnpauseSubmit)
	sfe.handleWithTimeout(mux, "GET "+unpauseStatus, sfe.UnpauseStatus)

	return measured_http.New(mux, sfe.clk, stats, oTelHTTPOptions...)
}

// renderTemplate takes the name of an HTML template and optional dynamicData
// which are rendered and served back to the client via the response writer.
func (sfe *SelfServiceFrontEndImpl) renderTemplate(w http.ResponseWriter, filename string, dynamicData any) {
	if len(filename) == 0 {
		http.Error(w, "Template page does not exist", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := sfe.templatePages.ExecuteTemplate(w, filename, dynamicData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Index is the homepage of the SFE
func (sfe *SelfServiceFrontEndImpl) Index(response http.ResponseWriter, request *http.Request) {
	sfe.renderTemplate(response, "index.html", nil)
}

// BuildID tells the requester what boulder build version is running.
func (sfe *SelfServiceFrontEndImpl) BuildID(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "text/plain")
	response.WriteHeader(http.StatusOK)
	detailsString := fmt.Sprintf("Boulder=(%s %s)", core.GetBuildID(), core.GetBuildTime())
	if _, err := fmt.Fprintln(response, detailsString); err != nil {
		sfe.log.Warningf("Could not write response: %s", err)
	}
}

// UnpauseForm allows a requester to unpause their account via a form present on
// the page. The Subscriber's client will receive a log line emitted by the WFE
// which contains a URL pre-filled with a JWT that will populate a hidden field
// in this form.
func (sfe *SelfServiceFrontEndImpl) UnpauseForm(response http.ResponseWriter, request *http.Request) {
	incomingJWT := request.URL.Query().Get("jwt")

	accountID, idents, err := sfe.parseUnpauseJWT(incomingJWT)
	if err != nil {
		if errors.Is(err, jwt.ErrExpired) {
			// JWT expired before the Subscriber visited the unpause page.
			sfe.unpauseTokenExpired(response)
			return
		}
		if errors.Is(err, unpause.ErrMalformedJWT) {
			// JWT is malformed. This could happen if the Subscriber failed to
			// copy the entire URL from their logs.
			sfe.unpauseRequestMalformed(response)
			return
		}
		sfe.unpauseFailed(response)
		return
	}

	// If any of these values change, ensure any relevant pages in //sfe/pages/
	// are also updated.
	type tmplData struct {
		PostPath  string
		JWT       string
		AccountID int64
		Idents    []string
	}

	// Present the unpause form to the Subscriber.
	sfe.renderTemplate(response, "unpause-form.html", tmplData{unpausePostForm, incomingJWT, accountID, idents})
}

// UnpauseSubmit serves a page showing the result of the unpause form submission.
// CSRF is not addressed because a third party causing submission of an unpause
// form is not harmful.
func (sfe *SelfServiceFrontEndImpl) UnpauseSubmit(response http.ResponseWriter, request *http.Request) {
	incomingJWT := request.URL.Query().Get("jwt")

	accountID, _, err := sfe.parseUnpauseJWT(incomingJWT)
	if err != nil {
		if errors.Is(err, jwt.ErrExpired) {
			// JWT expired before the Subscriber could click the unpause button.
			sfe.unpauseTokenExpired(response)
			return
		}
		if errors.Is(err, unpause.ErrMalformedJWT) {
			// JWT is malformed. This should never happen if the request came
			// from our form.
			sfe.unpauseRequestMalformed(response)
			return
		}
		sfe.unpauseFailed(response)
		return
	}

	unpaused, err := sfe.ra.UnpauseAccount(request.Context(), &rapb.UnpauseAccountRequest{
		RegistrationID: accountID,
	})
	if err != nil {
		sfe.unpauseFailed(response)
		return
	}

	// Redirect to the unpause status page with the count of unpaused
	// identifiers.
	params := url.Values{}
	params.Add("count", fmt.Sprintf("%d", unpaused.Count))
	http.Redirect(response, request, unpauseStatus+"?"+params.Encode(), http.StatusFound)
}

func (sfe *SelfServiceFrontEndImpl) unpauseRequestMalformed(response http.ResponseWriter) {
	sfe.renderTemplate(response, "unpause-invalid-request.html", nil)
}

func (sfe *SelfServiceFrontEndImpl) unpauseTokenExpired(response http.ResponseWriter) {
	sfe.renderTemplate(response, "unpause-expired.html", nil)
}

type unpauseStatusTemplate struct {
	Successful bool
	Limit      int64
	Count      int64
}

func (sfe *SelfServiceFrontEndImpl) unpauseFailed(response http.ResponseWriter) {
	sfe.renderTemplate(response, "unpause-status.html", unpauseStatusTemplate{Successful: false})
}

func (sfe *SelfServiceFrontEndImpl) unpauseSuccessful(response http.ResponseWriter, count int64) {
	sfe.renderTemplate(response, "unpause-status.html", unpauseStatusTemplate{
		Successful: true,
		Limit:      unpause.RequestLimit,
		Count:      count},
	)
}

// UnpauseStatus displays a success message to the Subscriber indicating that
// their account has been unpaused.
func (sfe *SelfServiceFrontEndImpl) UnpauseStatus(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodHead && request.Method != http.MethodGet {
		response.Header().Set("Access-Control-Allow-Methods", "GET, HEAD")
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	count, err := strconv.ParseInt(request.URL.Query().Get("count"), 10, 64)
	if err != nil || count < 0 {
		sfe.unpauseFailed(response)
		return
	}

	sfe.unpauseSuccessful(response, count)
}

// parseUnpauseJWT extracts and returns the subscriber's registration ID and a
// slice of paused identifiers from the claims. If the JWT cannot be parsed or
// is otherwise invalid, an error is returned. If the JWT is missing or
// malformed, unpause.ErrMalformedJWT is returned.
func (sfe *SelfServiceFrontEndImpl) parseUnpauseJWT(incomingJWT string) (int64, []string, error) {
	if incomingJWT == "" || len(strings.Split(incomingJWT, ".")) != 3 {
		// JWT is missing or malformed. This could happen if the Subscriber
		// failed to copy the entire URL from their logs. This should never
		// happen if the request came from our form.
		return 0, nil, unpause.ErrMalformedJWT
	}

	claims, err := unpause.RedeemJWT(incomingJWT, sfe.unpauseHMACKey, unpause.APIVersion, sfe.clk)
	if err != nil {
		return 0, nil, err
	}

	account, convErr := strconv.ParseInt(claims.Subject, 10, 64)
	if convErr != nil {
		// This should never happen as this was just validated by the call to
		// unpause.RedeemJWT().
		return 0, nil, errors.New("failed to parse account ID from JWT")
	}

	return account, strings.Split(claims.I, ","), nil
}
