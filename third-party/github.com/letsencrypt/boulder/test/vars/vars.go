package vars

import "fmt"

const (
	dbURL = "%s@tcp(boulder-proxysql:6033)/%s"
)

var (
	// DBConnSA is the sa database connection
	DBConnSA = fmt.Sprintf(dbURL, "sa", "boulder_sa_test")
	// DBConnSAMailer is the sa mailer database connection
	DBConnSAMailer = fmt.Sprintf(dbURL, "mailer", "boulder_sa_test")
	// DBConnSAFullPerms is the sa database connection with full perms
	DBConnSAFullPerms = fmt.Sprintf(dbURL, "test_setup", "boulder_sa_test")
	// DBConnSAIntegrationFullPerms is the sa database connection for the
	// integration test DB, with full perms
	DBConnSAIntegrationFullPerms = fmt.Sprintf(dbURL, "test_setup", "boulder_sa_integration")
	// DBInfoSchemaRoot is the root user and the information_schema connection.
	DBInfoSchemaRoot = fmt.Sprintf(dbURL, "root", "information_schema")
	// DBConnIncidents is the incidents database connection.
	DBConnIncidents = fmt.Sprintf(dbURL, "incidents_sa", "incidents_sa_test")
	// DBConnIncidentsFullPerms is the incidents database connection with full perms.
	DBConnIncidentsFullPerms = fmt.Sprintf(dbURL, "test_setup", "incidents_sa_test")
)
