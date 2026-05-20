// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package store provides the persistent storage layer for the metrics.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	pkgmetricsrecorder "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/recorder"
)

const (
	schemaVersion = "v0_5"

	// columnUnixMilliseconds represents the Unix timestamp of the metric.
	columnUnixMilliseconds = "unix_milliseconds"

	// columnComponentName represents the name of the component this metric
	// belongs to.
	columnComponentName = "component_name"

	// columnMetricName represents the name of the metric.
	columnMetricName = "metric_name"

	// columnMetricLabels represents the labels of the metric
	// such as GPU ID, mount points, etc. (as a secondary metric name).
	// The value is a set of key-value pairs in JSON format.
	//
	// Go JSON encoder sorts the keys alphabetically.
	// ref. https://pkg.go.dev/encoding/json#Marshal
	columnMetricLabels = "metric_labels"

	// columnMetricValue represents the numeric value of the metric.
	columnMetricValue = "metric_value"

	// columnMetricType represents the Prometheus metric type.
	columnMetricType = "metric_type"
)

// TODO: drop the old table "gpud_metrics"

// DefaultTableName is the default table name for the metrics.
var DefaultTableName = fmt.Sprintf("gpud_metrics_%s", schemaVersion)

var (
	ErrEmptyTableName     = errors.New("table name is empty")
	ErrEmptyComponentName = errors.New("component name is empty")
	ErrEmptyMetricName    = errors.New("metric name is empty")
)

var _ pkgmetrics.Store = &sqliteStore{}

var sqliteIdentifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type sqliteStore struct {
	dbRW  *sql.DB
	dbRO  *sql.DB
	table string
}

func NewSQLiteStore(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, table string) (pkgmetrics.Store, error) {
	if err := CreateTable(ctx, dbRW, table); err != nil {
		return nil, err
	}
	return &sqliteStore{
		dbRW:  dbRW,
		dbRO:  dbRO,
		table: table,
	}, nil
}

func (s *sqliteStore) Record(ctx context.Context, ms ...pkgmetrics.Metric) error {
	return insert(ctx, s.dbRW, s.table, ms...)
}

func (s *sqliteStore) Read(ctx context.Context, opts ...pkgmetrics.OpOption) (pkgmetrics.Metrics, error) {
	return read(ctx, s.dbRO, s.table, opts...)
}

func (s *sqliteStore) Purge(ctx context.Context, before time.Time) (int, error) {
	return purge(ctx, s.dbRW, s.table, before)
}

func CreateTable(ctx context.Context, dbRW *sql.DB, table string) error {
	if table == "" {
		return ErrEmptyTableName
	}

	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s REAL NOT NULL,
	%s TEXT NOT NULL DEFAULT '%s',
	PRIMARY KEY (%s, %s, %s, %s)
) WITHOUT ROWID;`,
		table,
		columnUnixMilliseconds, columnComponentName, columnMetricName, columnMetricLabels, columnMetricValue, columnMetricType, pkgmetrics.MetricTypeGauge, // columns
		columnUnixMilliseconds, columnComponentName, columnMetricName, columnMetricLabels, // primary keys
	))
	if err != nil {
		return err
	}
	return ensureMetricTypeColumn(ctx, dbRW, table)
}

func insert(ctx context.Context, dbRW *sql.DB, table string, ms ...pkgmetrics.Metric) error {
	if table == "" {
		return ErrEmptyTableName
	}

	if len(ms) == 0 {
		return nil
	}

	// Validate all metrics first
	for _, m := range ms {
		if m.Component == "" {
			return ErrEmptyComponentName
		}
		if m.Name == "" {
			return ErrEmptyMetricName
		}
	}

	// Build the query with placeholders for all metrics
	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s (%s, %s, %s, %s, %s, %s) VALUES ",
		table,
		columnUnixMilliseconds,
		columnComponentName,
		columnMetricName,
		columnMetricLabels,
		columnMetricValue,
		columnMetricType,
	)

	// Create proper placeholders with commas between value sets
	placeholders := make([]string, len(ms))
	for i := range placeholders {
		placeholders[i] = "(?, ?, ?, ?, ?, ?)"
	}
	query += strings.Join(placeholders, ", ")

	args := make([]interface{}, 0, len(ms)*6)
	for _, m := range ms {
		labels := ""
		if len(m.Labels) > 0 {
			b, err := json.Marshal(m.Labels)
			if err != nil {
				return err
			}
			labels = string(b)
		}
		metricType := m.Type
		if metricType == "" {
			metricType = pkgmetrics.MetricTypeGauge
		}
		args = append(args, m.UnixMilliseconds, m.Component, m.Name, labels, m.Value, metricType)
	}

	log.Logger.Infow("inserting metrics", "metrics", len(ms))
	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, args...)
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	return err
}

// read returns the metric data in the ascending order of unix seconds
// meaning the first element is the oldest event.
// It returns nil if no record is found ("database/sql.ErrNoRows").
func read(ctx context.Context, dbRO *sql.DB, table string, opts ...pkgmetrics.OpOption) (pkgmetrics.Metrics, error) {
	op := &pkgmetrics.Op{}
	if err := op.ApplyOpts(opts); err != nil {
		return nil, err
	}

	if table == "" {
		return nil, ErrEmptyTableName
	}

	params := []any{}
	if !op.Since.IsZero() {
		params = append(params, op.Since.UnixMilli())
	}

	orderByStatement := fmt.Sprintf("ORDER BY %s ASC;", columnUnixMilliseconds)
	whereStatement := ""
	if !op.Since.IsZero() {
		whereStatement = fmt.Sprintf("%s >= ?", columnUnixMilliseconds)
	}
	if len(op.SelectedComponents) > 0 {
		if whereStatement != "" {
			whereStatement += " AND "
		}

		placeholders := make([]string, 0, len(op.SelectedComponents))
		for component := range op.SelectedComponents {
			placeholders = append(placeholders, "?")
			params = append(params, component)
		}
		whereStatement += fmt.Sprintf("%s IN (%s)", columnComponentName, strings.Join(placeholders, ", "))
	}
	if whereStatement != "" {
		whereStatement = fmt.Sprintf("WHERE %s", whereStatement)
	}

	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s
FROM %s
`,
		columnUnixMilliseconds,
		columnComponentName,
		columnMetricName,
		columnMetricLabels,
		columnMetricValue,
		columnMetricType,
		table,
	)
	if whereStatement != "" {
		query += whereStatement + "\n"
	}
	query += orderByStatement

	start := time.Now()
	defer func() {
		pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())
	}()

	queryRows, err := dbRO.QueryContext(ctx, query, params...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		_ = queryRows.Close()
	}()

	rows := make(pkgmetrics.Metrics, 0)
	for queryRows.Next() {
		m := pkgmetrics.Metric{}
		var labels sql.NullString
		if err := queryRows.Scan(&m.UnixMilliseconds, &m.Component, &m.Name, &labels, &m.Value, &m.Type); err != nil {
			return nil, err
		}
		if m.Type == "" {
			m.Type = pkgmetrics.MetricTypeGauge
		}
		if labels.Valid && labels.String != "" {
			lm := make(map[string]string, 0)
			if err := json.Unmarshal([]byte(labels.String), &lm); err != nil {
				return nil, err
			}
			m.Labels = lm
		}
		rows = append(rows, m)
	}
	if err := queryRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func ensureMetricTypeColumn(ctx context.Context, dbRW *sql.DB, table string) error {
	quotedTable, err := quoteSQLiteIdentifier(table)
	if err != nil {
		return err
	}

	rows, err := dbRW.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s);", quotedTable))
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == columnMetricType {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s TEXT NOT NULL DEFAULT '%s';",
		quotedTable,
		columnMetricType,
		pkgmetrics.MetricTypeGauge,
	))
	return err
}

func quoteSQLiteIdentifier(name string) (string, error) {
	if name == "" {
		return "", ErrEmptyTableName
	}
	if !sqliteIdentifierRE.MatchString(name) {
		return "", fmt.Errorf("invalid sqlite identifier %q", name)
	}
	return `"` + name + `"`, nil
}

// purge purges the data for the corresponding component that is older
// than the given time.
func purge(ctx context.Context, dbRW *sql.DB, table string, before time.Time) (int, error) {
	if table == "" {
		return 0, ErrEmptyTableName
	}

	query := fmt.Sprintf(`
DELETE FROM %s WHERE %s < ?;`, table, columnUnixMilliseconds)

	start := time.Now()
	rs, err := dbRW.ExecContext(ctx, query, before.UnixMilli())
	pkgmetricsrecorder.RecordSQLiteDelete(time.Since(start).Seconds())

	if err != nil {
		return 0, err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
