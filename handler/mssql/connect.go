package mssql

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/ssh"
)

const defaultPort = 1433

func connect(ctx context.Context, uri string) (*sql.DB, *ssh.OpenResult, error) {
	open, err := ssh.MaybeOpenTunnel(uri, defaultPort)
	if err != nil {
		return nil, nil, err
	}
	dsn, err := uriToDSN(open.URI)
	if err != nil {
		_ = open.Close()
		return nil, nil, err
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		_ = open.Close()
		return nil, nil, errs.Wrap(err, "open mssql database")
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = open.Close()
		return nil, nil, errs.Wrap(err, "ping mssql database")
	}
	return db, open, nil
}

func uriToDSN(raw string) (string, error) {
	if isADOConnString(raw) {
		return ensureADOParams(raw), nil
	}
	if strings.HasPrefix(raw, "sqlserver://") || strings.HasPrefix(raw, "mssql://") {
		return ensureDSNParams(raw), nil
	}
	if strings.Contains(raw, "://") && !strings.HasPrefix(raw, "sqlserver://") && !strings.HasPrefix(raw, "mssql://") {
		return "", errs.New("unsupported mssql uri scheme: " + raw)
	}
	if !strings.Contains(raw, "://") {
		raw = "sqlserver://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errs.Wrap(err, "parse mssql uri")
	}
	if u.Scheme == "mssql" {
		u.Scheme = "sqlserver"
	}
	if u.User == nil {
		return "", errs.New("mssql uri requires a username")
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = "1433"
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", errs.New("mssql uri requires a database name")
	}

	q := u.Query()
	if q.Get("database") == "" {
		q.Set("database", dbName)
	}
	if q.Get("encrypt") == "" {
		q.Set("encrypt", "disable")
	}
	u.Host = net.JoinHostPort(host, port)
	u.Path = ""
	u.RawQuery = q.Encode()
	u.User = url.UserPassword(user, pass)
	return u.String(), nil
}

func isADOConnString(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "server=") || strings.Contains(lower, "data source=")
}

func ensureADOParams(dsn string) string {
	lower := strings.ToLower(dsn)
	if !strings.Contains(lower, "encrypt=") {
		sep := ";"
		if !strings.HasSuffix(strings.TrimSpace(dsn), ";") {
			dsn += sep
		}
		dsn += "encrypt=disable"
	}
	return dsn
}

func ensureDSNParams(dsn string) string {
	if strings.HasPrefix(dsn, "mssql://") {
		dsn = "sqlserver://" + strings.TrimPrefix(dsn, "mssql://")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	if q.Get("encrypt") == "" {
		q.Set("encrypt", "disable")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func querySQL(spec TableSpec) string {
	if spec.Query != "" {
		return spec.Query
	}
	return "SELECT * FROM " + quoteTable(spec.Name)
}
