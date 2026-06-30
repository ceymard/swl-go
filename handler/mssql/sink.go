package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

const insertBatchSize = 512

// Sink writes collections into Microsoft SQL Server using batched INSERT/MERGE.
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.URI == "" {
		return errs.New("mssql sink requires a connection URI")
	}
	h := &sinkHooks{cfg: cfg, opts: opts}
	return handlers.ConsumeHooks(cfg, h, in)
}

type sinkHooks struct {
	cfg  handlers.Config
	opts SinkOpts
	db   *sql.DB
	tun  interface{ Close() error }
	tx   *sql.Tx
}

func (h *sinkHooks) Init(ctx context.Context) error {
	db, tun, err := connect(ctx, h.opts.URI)
	if err != nil {
		return err
	}
	h.db = db
	h.tun = tun
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		_ = tun.Close()
		return errs.Wrap(err, "mssql begin")
	}
	h.tx = tx
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(1, "connected to mssql", h.opts.URI)
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	table := col.Name
	cols := columnNames(firstRow)

	if h.opts.Drop {
		stmt := "DROP TABLE IF EXISTS " + quoteTable(table)
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(2, "dropping table", table)
			h.cfg.Messages.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "drop table", "table", table)
		}
	}

	if h.opts.AutoCreate || h.opts.Drop {
		ddl := buildCreateTable(table, cols, firstRow)
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(3, ddl)
		}
		if _, err := h.tx.ExecContext(ctx, ddl); err != nil {
			return nil, errs.Wrap(err, "create table", "table", table)
		}
	}

	if h.opts.Truncate {
		stmt := "TRUNCATE TABLE " + quoteTable(table)
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(2, "truncating table", table)
			h.cfg.Messages.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "truncate table", "table", table)
		}
	}

	var pkCols []string
	identityCols, err := listIdentityColumns(ctx, h.tx, table)
	if err != nil {
		return nil, err
	}
	if h.opts.Upsert {
		pkCols, err = primaryKeyColumns(ctx, h.tx, table)
		if err != nil {
			return nil, err
		}
	}

	w := &batchWriter{
		tx:           h.tx,
		cfg:          h.cfg,
		table:        table,
		cols:         cols,
		upsert:       h.opts.Upsert,
		pkCols:       pkCols,
		identityCols: identityCols,
	}
	if err := w.append(firstRow); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *sinkHooks) Rollback(ctx context.Context) {
	if h.tx != nil {
		_ = h.tx.Rollback()
	}
	if h.db != nil {
		_ = h.db.Close()
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "rollbacked mssql transaction")
	}
}

func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.tx != nil {
		if err := h.tx.Commit(); err != nil {
			return errs.Wrap(err, "mssql commit")
		}
	}
	if h.db != nil {
		if err := h.db.Close(); err != nil {
			return errs.Wrap(err, "close mssql database")
		}
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "committed mssql changes")
	}
	return nil
}

type batchWriter struct {
	tx           *sql.Tx
	cfg          handlers.Config
	table        string
	cols         []string
	upsert       bool
	pkCols       []string
	identityCols []string
	pending      []coll.Row
	args         []any
}

func (w *batchWriter) Write(row coll.Row) error {
	return w.append(row)
}

func (w *batchWriter) Close() error {
	if err := w.flush(); err != nil {
		return err
	}
	if w.cfg.Messages != nil && w.cfg.Verbose >= 2 {
		var n int64
		q := "SELECT count(*) FROM " + quoteTable(w.table)
		if err := w.tx.QueryRowContext(context.Background(), q).Scan(&n); err == nil {
			w.cfg.Messages.Log(2, "table", w.table, "now has", n, "rows")
		}
	}
	return nil
}

func (w *batchWriter) append(row coll.Row) error {
	w.pending = append(w.pending, row)
	if len(w.pending) >= insertBatchSize {
		return w.flush()
	}
	return nil
}

func (w *batchWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}

	quotedCols := make([]string, len(w.cols))
	for i, c := range w.cols {
		quotedCols[i] = quoteColumn(c)
	}

	needArgs := len(w.cols) * len(w.pending)
	if cap(w.args) < needArgs {
		w.args = make([]any, needArgs)
	}
	args := w.args[:needArgs]

	argNum := 1
	valueRows := make([]string, len(w.pending))
	for i, row := range w.pending {
		ph := make([]string, len(w.cols))
		for j, c := range w.cols {
			v, err := bindValue(row[c])
			if err != nil {
				return errs.Wrap(err, "bind value", "column", c)
			}
			args[argNum-1] = v
			ph[j] = fmt.Sprintf("@p%d", argNum)
			argNum++
		}
		valueRows[i] = "(" + strings.Join(ph, ", ") + ")"
	}

	var stmt string
	if w.upsert && len(w.pkCols) > 0 {
		stmt = buildMergeStatement(w.table, w.cols, w.pkCols, quotedCols, valueRows)
	} else {
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
			quoteTable(w.table),
			strings.Join(quotedCols, ", "),
			strings.Join(valueRows, ", "),
		)
	}
	if w.cfg.Messages != nil {
		w.cfg.Messages.Log(3, stmt)
	}
	if needsIdentityInsert(w.cols, w.identityCols) {
		on := fmt.Sprintf("SET IDENTITY_INSERT %s ON", quoteTable(w.table))
		if _, err := w.tx.ExecContext(context.Background(), on); err != nil {
			return errs.Wrap(err, "enable identity insert", "table", w.table)
		}
		defer func() {
			off := fmt.Sprintf("SET IDENTITY_INSERT %s OFF", quoteTable(w.table))
			_, _ = w.tx.ExecContext(context.Background(), off)
		}()
	}
	if _, err := w.tx.ExecContext(context.Background(), stmt, args...); err != nil {
		return errs.Wrap(err, "insert batch", "table", w.table)
	}
	w.pending = w.pending[:0]
	return nil
}

func buildMergeStatement(table string, cols, pkCols, quotedCols, valueRows []string) string {
	srcCols := make([]string, len(cols))
	for i, c := range cols {
		srcCols[i] = "S." + quoteColumn(c)
	}
	onParts := make([]string, len(pkCols))
	for i, c := range pkCols {
		qc := quoteColumn(c)
		onParts[i] = "T." + qc + " = S." + qc
	}
	updateCols := make([]string, 0, len(cols))
	for _, c := range cols {
		if containsString(pkCols, c) {
			continue
		}
		qc := quoteColumn(c)
		updateCols = append(updateCols, "T."+qc+" = S."+qc)
	}

	var updateClause string
	if len(updateCols) > 0 {
		updateClause = "WHEN MATCHED THEN UPDATE SET " + strings.Join(updateCols, ", ")
	}

	return fmt.Sprintf(`MERGE %s WITH (HOLDLOCK) AS T
USING (VALUES %s) AS S (%s)
ON %s
%s
WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s);`,
		quoteTable(table),
		strings.Join(valueRows, ", "),
		strings.Join(quotedCols, ", "),
		strings.Join(onParts, " AND "),
		updateClause,
		strings.Join(quotedCols, ", "),
		strings.Join(srcCols, ", "),
	)
}

func buildCreateTable(table string, cols []string, sample coll.Row) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = quoteColumn(c) + " " + inferColumnType(sample[c])
	}
	return fmt.Sprintf("IF OBJECT_ID(%s, 'U') IS NULL CREATE TABLE %s (%s)",
		quoteLiteral(table), quoteTable(table), strings.Join(parts, ", "))
}

func quoteLiteral(s string) string {
	return "N'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func primaryKeyColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	schema, name := splitTable(table)
	rows, err := tx.QueryContext(ctx, `
		SELECT c.name
		FROM sys.indexes i
		INNER JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
		INNER JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		INNER JOIN sys.tables t ON i.object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		WHERE i.is_primary_key = 1
		  AND s.name = @p1
		  AND t.name = @p2
		ORDER BY ic.key_ordinal`, schema, name)
	if err != nil {
		return nil, errs.Wrap(err, "list primary key columns", "table", table)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, errs.Wrap(err, "scan primary key column")
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func listIdentityColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	schema, name := splitTable(table)
	rows, err := tx.QueryContext(ctx, `
		SELECT c.name
		FROM sys.identity_columns ic
		INNER JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		INNER JOIN sys.tables t ON ic.object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		WHERE s.name = @p1 AND t.name = @p2
		ORDER BY ic.column_id`, schema, name)
	if err != nil {
		return nil, errs.Wrap(err, "list identity columns", "table", table)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, errs.Wrap(err, "scan identity column")
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func needsIdentityInsert(cols, identityCols []string) bool {
	if len(identityCols) == 0 {
		return false
	}
	for _, c := range cols {
		if containsString(identityCols, c) {
			return true
		}
	}
	return false
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
