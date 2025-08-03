package sa

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

type dbMetricsCollector struct {
	db         *sql.DB
	dbSettings DbSettings

	maxOpenConns      *prometheus.Desc
	maxIdleConns      *prometheus.Desc
	connMaxLifetime   *prometheus.Desc
	connMaxIdleTime   *prometheus.Desc
	openConns         *prometheus.Desc
	inUse             *prometheus.Desc
	idle              *prometheus.Desc
	waitCount         *prometheus.Desc
	waitDuration      *prometheus.Desc
	maxIdleClosed     *prometheus.Desc
	maxLifetimeClosed *prometheus.Desc
}

// Describe is implemented with DescribeByCollect. That's possible because the
// Collect method will always return the same metrics with the same descriptors.
func (dbc dbMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(dbc, ch)
}

// Collect first triggers the dbMaps's sql.Db's Stats function. Then it
// creates constant metrics for each DBStats value on the fly based on the
// returned data.
//
// Note that Collect could be called concurrently, so we depend on
// Stats() to be concurrency-safe.
func (dbc dbMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	writeStat := func(stat *prometheus.Desc, typ prometheus.ValueType, val float64) {
		ch <- prometheus.MustNewConstMetric(stat, typ, val)
	}
	writeCounter := func(stat *prometheus.Desc, val float64) {
		writeStat(stat, prometheus.CounterValue, val)
	}
	writeGauge := func(stat *prometheus.Desc, val float64) {
		writeStat(stat, prometheus.GaugeValue, val)
	}

	// Translate the DBMap's db.DBStats counter values into Prometheus metrics.
	dbMapStats := dbc.db.Stats()
	writeGauge(dbc.maxOpenConns, float64(dbMapStats.MaxOpenConnections))
	writeGauge(dbc.maxIdleConns, float64(dbc.dbSettings.MaxIdleConns))
	writeGauge(dbc.connMaxLifetime, float64(dbc.dbSettings.ConnMaxLifetime))
	writeGauge(dbc.connMaxIdleTime, float64(dbc.dbSettings.ConnMaxIdleTime))
	writeGauge(dbc.openConns, float64(dbMapStats.OpenConnections))
	writeGauge(dbc.inUse, float64(dbMapStats.InUse))
	writeGauge(dbc.idle, float64(dbMapStats.Idle))
	writeCounter(dbc.waitCount, float64(dbMapStats.WaitCount))
	writeCounter(dbc.waitDuration, dbMapStats.WaitDuration.Seconds())
	writeCounter(dbc.maxIdleClosed, float64(dbMapStats.MaxIdleClosed))
	writeCounter(dbc.maxLifetimeClosed, float64(dbMapStats.MaxLifetimeClosed))
}

// initDBMetrics will register a Collector that translates the provided dbMap's
// stats and DbSettings into Prometheus metrics on the fly. The exported metrics
// all start with `db_`. The underlying data comes from sql.DBStats:
// https://pkg.go.dev/database/sql#DBStats
func initDBMetrics(db *sql.DB, stats prometheus.Registerer, dbSettings DbSettings, address string, user string) error {
	// Create a dbMetricsCollector and register it
	dbc := dbMetricsCollector{db: db, dbSettings: dbSettings}

	labels := prometheus.Labels{"address": address, "user": user}

	dbc.maxOpenConns = prometheus.NewDesc(
		"db_max_open_connections",
		"Maximum number of DB connections allowed.",
		nil, labels)

	dbc.maxIdleConns = prometheus.NewDesc(
		"db_max_idle_connections",
		"Maximum number of idle DB connections allowed.",
		nil, labels)

	dbc.connMaxLifetime = prometheus.NewDesc(
		"db_connection_max_lifetime",
		"Maximum lifetime of DB connections allowed.",
		nil, labels)

	dbc.connMaxIdleTime = prometheus.NewDesc(
		"db_connection_max_idle_time",
		"Maximum lifetime of idle DB connections allowed.",
		nil, labels)

	dbc.openConns = prometheus.NewDesc(
		"db_open_connections",
		"Number of established DB connections (in-use and idle).",
		nil, labels)

	dbc.inUse = prometheus.NewDesc(
		"db_inuse",
		"Number of DB connections currently in use.",
		nil, labels)

	dbc.idle = prometheus.NewDesc(
		"db_idle",
		"Number of idle DB connections.",
		nil, labels)

	dbc.waitCount = prometheus.NewDesc(
		"db_wait_count",
		"Total number of DB connections waited for.",
		nil, labels)

	dbc.waitDuration = prometheus.NewDesc(
		"db_wait_duration_seconds",
		"The total time blocked waiting for a new connection.",
		nil, labels)

	dbc.maxIdleClosed = prometheus.NewDesc(
		"db_max_idle_closed",
		"Total number of connections closed due to SetMaxIdleConns.",
		nil, labels)

	dbc.maxLifetimeClosed = prometheus.NewDesc(
		"db_max_lifetime_closed",
		"Total number of connections closed due to SetConnMaxLifetime.",
		nil, labels)

	return stats.Register(dbc)
}
