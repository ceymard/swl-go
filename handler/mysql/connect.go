package mysql

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/ssh"
)

const defaultPort = 3306

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
		return nil, nil, errs.Wrap(err, "open mysql database")
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = open.Close()
		return nil, nil, errs.Wrap(err, "ping mysql database")
	}
	return db, open, nil
}

func uriToDSN(raw string) (string, error) {
	if strings.Contains(raw, "@tcp(") {
		return ensureDSNParams(raw), nil
	}
	if !strings.Contains(raw, "://") {
		raw = "mysql://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errs.Wrap(err, "parse mysql uri")
	}
	if u.User == nil {
		return "", errs.New("mysql uri requires a username")
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", errs.New("mysql uri requires a database name")
	}
	q := u.Query()
	params := map[string]string{"charset": "utf8mb4"}
	for k, vs := range q {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	cfg := mysqldriver.Config{
		User:                 user,
		Passwd:               pass,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(host, port),
		DBName:               dbName,
		ParseTime:            true,
		AllowNativePasswords: true,
		Params:               params,
	}
	return cfg.FormatDSN(), nil
}

func ensureDSNParams(dsn string) string {
	if strings.Contains(dsn, "parseTime=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "parseTime=true&charset=utf8mb4"
}

func quoteTable(name string) string {
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = "`" + strings.ReplaceAll(p, "`", "``") + "`"
	}
	return strings.Join(parts, ".")
}

func querySQL(spec TableSpec) string {
	if spec.Query != "" {
		return spec.Query
	}
	return "SELECT * FROM " + quoteTable(spec.Name)
}
