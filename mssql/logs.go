package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"docklog/db"

	_ "github.com/microsoft/go-mssqldb"
)

var identRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const (
	MaxLogBatchSize     = 10000
	DefaultLogBatchSize = 10000
)

func ClampBatchLimit(limit int) int {
	if limit <= 0 {
		return DefaultLogBatchSize
	}
	if limit > MaxLogBatchSize {
		return MaxLogBatchSize
	}
	return limit
}

type LogFilter struct {
	ContainerName string
}

type LogRow struct {
	ID            int64
	Timestamp     time.Time
	ContainerName string
	Stream        string
	Message       string
}

type LogEntry struct {
	ID            int64  `json:"id"`
	TS            string `json:"ts,omitempty"`
	ContainerName string `json:"container,omitempty"`
	Stream        string `json:"stream,omitempty"`
	Message       string `json:"message"`
}

func RowToEntry(row LogRow) LogEntry {
	e := LogEntry{
		ID:            row.ID,
		ContainerName: row.ContainerName,
		Stream:        row.Stream,
		Message:       row.Message,
	}
	if !row.Timestamp.IsZero() {
		e.TS = row.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return e
}

func RowsToEntries(rows []LogRow) []LogEntry {
	out := make([]LogEntry, len(rows))
	for i, r := range rows {
		out[i] = RowToEntry(r)
	}
	return out
}

func bracketIdent(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !identRE.MatchString(name) {
		return "", fmt.Errorf("invalid SQL identifier: %q", name)
	}
	return "[" + name + "]", nil
}

func OpenDB(ctx context.Context, s db.MSSQLSettings) (dbConn *sql.DB, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("SQL Server driver error: %v", r)
			if dbConn != nil {
				dbConn.Close()
				dbConn = nil
			}
		}
	}()

	dbConn, err = openSQLDB(s)
	if err != nil {
		return nil, err
	}
	if err := dbConn.PingContext(ctx); err != nil {
		dbConn.Close()
		return nil, err
	}
	return dbConn, nil
}

func settingsFromStore() (db.MSSQLSettings, error) {
	s, err := db.GetMSSQLSettingsWithPassword()
	if err != nil {
		return s, err
	}
	db.ApplyMSSQLDefaults(&s)
	return s, nil
}

func qualifiedTable(s db.MSSQLSettings) (string, error) {
	table := strings.TrimSpace(s.TableName)
	if table == "" {
		return "", errors.New("table is required")
	}
	parts := strings.Split(table, ".")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		q, err := bracketIdent(p)
		if err != nil {
			return "", err
		}
		quoted = append(quoted, q)
	}
	if len(quoted) == 0 {
		return "", errors.New("table is required")
	}
	return strings.Join(quoted, "."), nil
}

type logColumns struct {
	idCol     string
	tsCol     string
	msgCol    string
	nameCol   string
	streamCol string
}

func resolveLogColumns(s db.MSSQLSettings) (logColumns, error) {
	var c logColumns
	var err error
	c.idCol, err = bracketIdent(s.IDColumn)
	if err != nil {
		return c, err
	}
	c.tsCol, err = bracketIdent(s.TimestampColumn)
	if err != nil {
		return c, err
	}
	c.msgCol, err = bracketIdent(s.MessageColumn)
	if err != nil {
		return c, err
	}
	c.nameCol, err = bracketIdent("container_name")
	if err != nil {
		return c, err
	}
	c.streamCol, err = bracketIdent("stream")
	if err != nil {
		return c, err
	}
	return c, nil
}

func containerFilterSQL(filter LogFilter) (clause string, containerParam interface{}) {
	if strings.TrimSpace(filter.ContainerName) == "" {
		return "", nil
	}
	return fmt.Sprintf(" AND %s = @container", "[container_name]"), strings.TrimSpace(filter.ContainerName)
}

const logTimeDisplay = "01-02-2006 15:04:05"

func logDisplayLocation() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}

func FormatLogLine(row LogRow) string {
	body := row.Message
	if row.ContainerName != "" {
		body = row.ContainerName + " | " + body
	}
	if row.Stream != "" {
		body = "[" + row.Stream + "] " + body
	}
	ts := row.Timestamp
	if ts.IsZero() {
		return body
	}
	return ts.In(logDisplayLocation()).Format(logTimeDisplay) + " ET " + body
}

func TestConnection(ctx context.Context) error {
	s, err := settingsFromStore()
	if err != nil {
		return err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return err
	}
	defer conn.Close()
	table, err := qualifiedTable(s)
	if err != nil {
		return err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return err
	}
	q := fmt.Sprintf("SELECT TOP 1 %s FROM %s ORDER BY %s DESC", cols.idCol, table, cols.idCol)
	var id sql.NullInt64
	return conn.QueryRowContext(ctx, q).Scan(&id)
}

func FetchTail(ctx context.Context, limit int, filter LogFilter) ([]LogRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return fetchWindow(ctx, limit, filter, 0, false)
}

func FetchBeforeID(ctx context.Context, beforeID int64, limit int, filter LogFilter) ([]LogRow, error) {
	return FetchBatch(ctx, limit, beforeID, filter)
}

func FetchBatch(ctx context.Context, limit int, beforeID int64, filter LogFilter) ([]LogRow, error) {
	limit = ClampBatchLimit(limit)

	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return nil, err
	}
	filterClause, containerParam := containerFilterSQL(filter)

	args := []interface{}{sql.Named("p1", limit)}
	whereBefore := ""
	if beforeID > 0 {
		whereBefore = fmt.Sprintf(" AND %s < @before_id", cols.idCol)
		args = append(args, sql.Named("before_id", beforeID))
	}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}

	q := fmt.Sprintf(`
		SELECT TOP (@p1)
			%s AS id, %s AS ts, %s AS cname, %s AS stream, %s AS msg
		FROM %s
		WHERE 1=1%s%s
		ORDER BY %s DESC`,
		cols.idCol, cols.tsCol, cols.nameCol, cols.streamCol, cols.msgCol,
		table, filterClause, whereBefore, cols.idCol,
	)

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

func FetchPage(ctx context.Context, page, limit int, filter LogFilter) ([]LogRow, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := (page - 1) * limit

	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return nil, err
	}
	filterClause, containerParam := containerFilterSQL(filter)

	args := []interface{}{
		sql.Named("offset", offset),
		sql.Named("limit", limit),
	}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}

	q := fmt.Sprintf(`
		SELECT %s AS id, %s AS ts, %s AS cname, %s AS stream, %s AS msg
		FROM %s
		WHERE 1=1%s
		ORDER BY %s DESC
		OFFSET @offset ROWS FETCH NEXT @limit ROWS ONLY`,
		cols.idCol, cols.tsCol, cols.nameCol, cols.streamCol, cols.msgCol,
		table, filterClause, cols.idCol,
	)

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

func FetchAll(ctx context.Context, filter LogFilter) ([]LogRow, error) {
	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return nil, err
	}
	filterClause, containerParam := containerFilterSQL(filter)

	args := []interface{}{}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}

	q := fmt.Sprintf(`
		SELECT %s AS id, %s AS ts, %s AS cname, %s AS stream, %s AS msg
		FROM %s
		WHERE 1=1%s
		ORDER BY %s DESC`,
		cols.idCol, cols.tsCol, cols.nameCol, cols.streamCol, cols.msgCol,
		table, filterClause, cols.idCol,
	)

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

func fetchWindow(ctx context.Context, limit int, filter LogFilter, beforeID int64, before bool) ([]LogRow, error) {
	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return nil, err
	}
	filterClause, containerParam := containerFilterSQL(filter)

	whereBefore := ""
	args := []interface{}{sql.Named("p1", limit)}
	if before {
		whereBefore = fmt.Sprintf(" AND %s < @before_id", cols.idCol)
		args = append(args, sql.Named("before_id", beforeID))
	}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}

	q := fmt.Sprintf(`
		SELECT id, ts, cname, stream, msg FROM (
			SELECT TOP (@p1)
				%s AS id, %s AS ts, %s AS cname, %s AS stream, %s AS msg
			FROM %s
			WHERE 1=1%s%s
			ORDER BY %s DESC
		) AS t ORDER BY id ASC`,
		cols.idCol, cols.tsCol, cols.nameCol, cols.streamCol, cols.msgCol,
		table, filterClause, whereBefore, cols.idCol,
	)

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

func FetchAfterID(ctx context.Context, afterID int64, limit int, filter LogFilter) ([]LogRow, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return nil, err
	}
	filterClause, containerParam := containerFilterSQL(filter)

	args := []interface{}{sql.Named("after_id", afterID)}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}

	q := fmt.Sprintf(`
		SELECT %s AS id, %s AS ts, %s AS cname, %s AS stream, %s AS msg
		FROM %s
		WHERE %s > @after_id%s
		ORDER BY %s ASC`,
		cols.idCol, cols.tsCol, cols.nameCol, cols.streamCol, cols.msgCol,
		table, cols.idCol, filterClause, cols.idCol,
	)

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanLogRows(rows)
	if err != nil {
		return nil, err
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func CountLogs(ctx context.Context, filter LogFilter) (int64, error) {
	s, err := settingsFromStore()
	if err != nil {
		return 0, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	table, err := qualifiedTable(s)
	if err != nil {
		return 0, err
	}
	cols, err := resolveLogColumns(s)
	if err != nil {
		return 0, err
	}
	filterClause, containerParam := containerFilterSQL(filter)
	args := []interface{}{}
	if containerParam != nil {
		args = append(args, sql.Named("container", containerParam))
	}
	q := fmt.Sprintf("SELECT COUNT(%s) FROM %s WHERE 1=1%s", cols.idCol, table, filterClause)
	var count int64
	err = conn.QueryRowContext(ctx, q, args...).Scan(&count)
	return count, err
}

func ListContainers(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}
	s, err := settingsFromStore()
	if err != nil {
		return nil, err
	}
	conn, err := OpenDB(ctx, s)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	table, err := qualifiedTable(s)
	if err != nil {
		return nil, err
	}
	nameCol, err := bracketIdent("container_name")
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT TOP (@p1) %s
		FROM %s
		WHERE %s IS NOT NULL AND LTRIM(RTRIM(%s)) <> ''
		ORDER BY %s ASC`, nameCol, table, nameCol, nameCol, nameCol)
	rows, err := conn.QueryContext(ctx, q, sql.Named("p1", limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 32)
	for rows.Next() {
		var name sql.NullString
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name.Valid {
			out = append(out, name.String)
		}
	}
	return out, rows.Err()
}

func scanLogRows(rows *sql.Rows) ([]LogRow, error) {
	out := make([]LogRow, 0, 64)
	for rows.Next() {
		var row LogRow
		var ts sql.NullTime
		var msg, cname, stream sql.NullString
		if err := rows.Scan(&row.ID, &ts, &cname, &stream, &msg); err != nil {
			return nil, err
		}
		if ts.Valid {
			row.Timestamp = ts.Time
		}
		if msg.Valid {
			row.Message = msg.String
		}
		if cname.Valid {
			row.ContainerName = cname.String
		}
		if stream.Valid {
			row.Stream = stream.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func RowsToLines(rows []LogRow) []string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = FormatLogLine(r)
	}
	return lines
}

func LogsPayload(rows []LogRow) map[string]interface{} {
	return LogsPayloadPage(rows, 0, 0, 0)
}

func LogsPayloadBatch(rows []LogRow, batchSize int, total int64) map[string]interface{} {
	batchSize = ClampBatchLimit(batchSize)
	out := LogsPayloadPage(rows, 0, batchSize, total)
	out["batch_size"] = batchSize
	hasMore := len(rows) >= batchSize
	if total > 0 && int64(len(rows)) >= total {
		hasMore = false
	}
	out["has_more_older"] = hasMore
	return out
}

func LogsPayloadPage(rows []LogRow, page, pageSize int, total int64) map[string]interface{} {
	lines := RowsToLines(rows)
	var minID, maxID *int64
	if len(rows) > 0 {
		min := rows[0].ID
		max := rows[0].ID
		for _, r := range rows[1:] {
			if r.ID < min {
				min = r.ID
			}
			if r.ID > max {
				max = r.ID
			}
		}
		minID = &min
		maxID = &max
	}
	out := map[string]interface{}{
		"lines":   lines,
		"entries": RowsToEntries(rows),
		"min_id":  minID,
		"max_id":  maxID,
	}
	if page > 0 {
		out["page"] = page
	}
	if pageSize > 0 {
		out["page_size"] = pageSize
	}
	if total > 0 {
		out["total"] = total
		tp := total / int64(pageSize)
		if total%int64(pageSize) != 0 {
			tp++
		}
		if tp < 1 {
			tp = 1
		}
		out["total_pages"] = tp
	}
	return out
}

func LogFilterFromQuery(container string) LogFilter {
	return LogFilter{ContainerName: strings.TrimSpace(container)}
}
