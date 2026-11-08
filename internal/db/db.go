// Package db creates and drops the database (and the user that owns it) of an application backend in PostgreSQL,
// Oracle or Microsoft SQL Server, and builds the connection details the backend receives.
package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
	_ "github.com/microsoft/go-mssqldb" // database/sql driver "sqlserver"
	go_ora "github.com/sijms/go-ora/v2"
)

// Engine names (the values of spec.engine).
const (
	PostgreSQL = "PostgreSQL"
	Oracle     = "Oracle"
	SQLServer  = "SQLServer"
)

// Target is the database to keep and the server it lives in.
type Target struct {
	Engine     string
	Host       string
	Port       int32
	Database   string // Oracle: the schema (a user)
	Owner      string // Oracle: always the schema
	Extensions []string
	SSLMode    string // PostgreSQL
	Service    string // Oracle: service name of the PDB
	Tablespace string // Oracle: quota tablespace
	Encrypt    string // SQL Server
	TrustCert  bool   // SQL Server
}

// Admin are the credentials of the administrator the operator connects with.
type Admin struct{ Username, Password string }

var (
	pgIdent  = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
	oraIdent = regexp.MustCompile(`^[a-z][a-z0-9_]{0,29}$`)
	sqlIdent = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,127}$`)
)

// Normalise fills the defaults of the engine and checks the names.
func (t *Target) Normalise() error {
	if t.Engine == "" {
		t.Engine = PostgreSQL
	}
	if t.Owner == "" || t.Engine == Oracle {
		t.Owner = t.Database
	}
	switch t.Engine {
	case PostgreSQL:
		if t.Port == 0 {
			t.Port = 5432
		}
		if t.SSLMode == "" {
			t.SSLMode = "prefer"
		}
		if !pgIdent.MatchString(t.Database) || !pgIdent.MatchString(t.Owner) {
			return fmt.Errorf("database and owner must be lower-case PostgreSQL identifiers: %q, %q", t.Database, t.Owner)
		}
	case Oracle:
		if t.Port == 0 {
			t.Port = 1521
		}
		if t.Service == "" {
			return fmt.Errorf("engine Oracle needs oracle.service (the service name of the pluggable database)")
		}
		if t.Tablespace == "" {
			t.Tablespace = "USERS"
		}
		if !oraIdent.MatchString(t.Database) {
			return fmt.Errorf("with Oracle the database is a schema: a lower-case identifier of up to 30 characters, got %q", t.Database)
		}
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_$#]{0,29}$`).MatchString(t.Tablespace) {
			return fmt.Errorf("invalid tablespace %q", t.Tablespace)
		}
	case SQLServer:
		if t.Port == 0 {
			t.Port = 1433
		}
		switch t.Encrypt {
		case "", "mandatory", "true":
			t.Encrypt = "true"
		case "optional", "false":
			t.Encrypt = "false"
		case "disable", "strict":
		default:
			return fmt.Errorf("invalid sqlServer.encrypt %q (disable, optional, mandatory or strict)", t.Encrypt)
		}
		if !sqlIdent.MatchString(t.Database) || !sqlIdent.MatchString(t.Owner) {
			return fmt.Errorf("database and owner must be SQL Server identifiers (letters, digits, _): %q, %q", t.Database, t.Owner)
		}
	default:
		return fmt.Errorf("unknown engine %q (PostgreSQL, Oracle or SQLServer)", t.Engine)
	}
	return nil
}

// Password generates a password that every engine accepts: 27 characters (Oracle allows 30), with upper and lower
// case letters and digits (the SQL Server complexity policy), and no quotes.
func Password() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return "Dx" + strings.NewReplacer("-", "x", "_", "y").Replace(base64.RawURLEncoding.EncodeToString(b)) + "9"
}

// Details are the connection details written to the Secret of the backend.
func (t Target) Details(password string) map[string]string {
	hostPort := fmt.Sprintf("%s:%d", t.Host, t.Port)
	d := map[string]string{"engine": t.Engine, "username": t.Owner, "password": password, "host": t.Host,
		"port": fmt.Sprint(t.Port), "database": t.Database}
	switch t.Engine {
	case PostgreSQL:
		d["uri"] = (&url.URL{Scheme: "postgresql", User: url.UserPassword(t.Owner, password), Host: hostPort, Path: "/" + t.Database, RawQuery: "sslmode=" + t.SSLMode}).String()
		d["jdbcUrl"] = fmt.Sprintf("jdbc:postgresql://%s/%s?sslmode=%s", hostPort, t.Database, t.SSLMode)
	case Oracle:
		d["service"] = t.Service
		d["uri"] = (&url.URL{Scheme: "oracle", User: url.UserPassword(strings.ToUpper(t.Owner), password), Host: hostPort, Path: "/" + t.Service}).String()
		d["jdbcUrl"] = fmt.Sprintf("jdbc:oracle:thin:@//%s/%s", hostPort, t.Service)
		d["username"] = strings.ToUpper(t.Owner)
	case SQLServer:
		q := url.Values{"database": {t.Database}, "encrypt": {t.Encrypt}}
		if t.TrustCert {
			q.Set("TrustServerCertificate", "true")
		}
		d["uri"] = (&url.URL{Scheme: "sqlserver", User: url.UserPassword(t.Owner, password), Host: hostPort, RawQuery: q.Encode()}).String()
		d["jdbcUrl"] = fmt.Sprintf("jdbc:sqlserver://%s;databaseName=%s;encrypt=%s;trustServerCertificate=%t", hostPort, t.Database, t.Encrypt, t.TrustCert)
	}
	return d
}

func (t Target) adminDSN(a Admin, dbname string) (driver, dsn string) {
	hostPort := fmt.Sprintf("%s:%d", t.Host, t.Port)
	switch t.Engine {
	case Oracle:
		return "oracle", go_ora.BuildUrl(t.Host, int(t.Port), t.Service, a.Username, a.Password, map[string]string{"CONNECTION TIMEOUT": "15"})
	case SQLServer:
		q := url.Values{"database": {"master"}, "encrypt": {t.Encrypt}, "dial timeout": {"15"}}
		if t.TrustCert {
			q.Set("TrustServerCertificate", "true")
		}
		return "sqlserver", (&url.URL{Scheme: "sqlserver", User: url.UserPassword(a.Username, a.Password), Host: hostPort, RawQuery: q.Encode()}).String()
	default:
		return "pgx", (&url.URL{Scheme: "postgres", User: url.UserPassword(a.Username, a.Password), Host: hostPort, Path: "/" + dbname,
			RawQuery: "sslmode=" + t.SSLMode + "&connect_timeout=10"}).String()
	}
}

func (t Target) open(ctx context.Context, a Admin, dbname string) (*sql.DB, error) {
	driver, dsn := t.adminDSN(a, dbname)
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := db.PingContext(cctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cannot connect to %s %s:%d as %s: %w", t.Engine, t.Host, t.Port, a.Username, err)
	}
	return db, nil
}

func pgQuote(s string) string  { return pgx.Identifier{s}.Sanitize() }
func literal(s string) string  { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func nliteral(s string) string { return "N" + literal(s) }
func bracket(s string) string  { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }

// Ensure creates or updates the owner (with this password) and creates the database if it is missing. It reports
// whether the database was created now.
func (t Target) Ensure(ctx context.Context, a Admin, password string) (bool, error) {
	switch t.Engine {
	case Oracle:
		return t.ensureOracle(ctx, a, password)
	case SQLServer:
		return t.ensureSQLServer(ctx, a, password)
	default:
		return t.ensurePostgres(ctx, a, password)
	}
}

func (t Target) ensurePostgres(ctx context.Context, a Admin, password string) (bool, error) {
	db, err := t.open(ctx, a, "postgres")
	if err != nil {
		return false, err
	}
	defer db.Close() //nolint:errcheck
	role := pgQuote(t.Owner)
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", t.Owner).Scan(&exists); err != nil {
		return false, err
	}
	stmt := "ALTER ROLE "
	if !exists {
		stmt = "CREATE ROLE "
	}
	// The password cannot be a bind parameter in CREATE/ALTER ROLE: it is quoted as a literal
	if _, err := db.ExecContext(ctx, stmt+role+" WITH LOGIN PASSWORD "+literal(password)); err != nil {
		return false, fmt.Errorf("role %s: %w", t.Owner, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", t.Database).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "CREATE DATABASE "+pgQuote(t.Database)+" OWNER "+role); err != nil {
			return false, fmt.Errorf("database %s: %w", t.Database, err)
		}
	}
	if len(t.Extensions) > 0 {
		dbc, err := t.open(ctx, a, t.Database)
		if err != nil {
			return !exists, err
		}
		defer dbc.Close() //nolint:errcheck
		for _, e := range t.Extensions {
			if _, err := dbc.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgQuote(e)); err != nil {
				return !exists, fmt.Errorf("extension %s: %w", e, err)
			}
		}
	}
	return !exists, nil
}

// oracleStatements are the statements that create (or update) the schema user and its privileges.
func (t Target) oracleStatements(exists bool, password string) []string {
	user := strings.ToUpper(t.Owner)
	pw := `"` + password + `"`
	if exists {
		return []string{"ALTER USER " + user + " IDENTIFIED BY " + pw + " ACCOUNT UNLOCK"}
	}
	return []string{
		"CREATE USER " + user + " IDENTIFIED BY " + pw + " DEFAULT TABLESPACE " + t.Tablespace + " QUOTA UNLIMITED ON " + t.Tablespace,
		"GRANT CREATE SESSION, CREATE TABLE, CREATE VIEW, CREATE SEQUENCE, CREATE PROCEDURE, CREATE TRIGGER, CREATE TYPE, CREATE SYNONYM TO " + user,
	}
}

func (t Target) ensureOracle(ctx context.Context, a Admin, password string) (bool, error) {
	db, err := t.open(ctx, a, "")
	if err != nil {
		return false, err
	}
	defer db.Close() //nolint:errcheck
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_users WHERE username = :1", strings.ToUpper(t.Owner)).Scan(&n); err != nil {
		return false, err
	}
	for _, s := range t.oracleStatements(n > 0, password) {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return false, fmt.Errorf("schema %s: %w", strings.ToUpper(t.Owner), err)
		}
	}
	return n == 0, nil
}

// sqlServerStatements create (or update) the login, the database, its user and its db_owner membership.
func (t Target) sqlServerStatements(password string) []string {
	login, dbn := bracket(t.Owner), bracket(t.Database)
	return []string{
		"IF NOT EXISTS (SELECT 1 FROM sys.server_principals WHERE name = " + nliteral(t.Owner) + ") CREATE LOGIN " + login +
			" WITH PASSWORD = " + nliteral(password) + ", CHECK_POLICY = ON ELSE ALTER LOGIN " + login + " WITH PASSWORD = " + nliteral(password),
		"IF DB_ID(" + nliteral(t.Database) + ") IS NULL CREATE DATABASE " + dbn,
		"USE " + dbn + "; IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = " + nliteral(t.Owner) + ") CREATE USER " + login +
			" FOR LOGIN " + login + "; ALTER ROLE db_owner ADD MEMBER " + login + ";",
	}
}

func (t Target) ensureSQLServer(ctx context.Context, a Admin, password string) (bool, error) {
	db, err := t.open(ctx, a, "")
	if err != nil {
		return false, err
	}
	defer db.Close() //nolint:errcheck
	var id sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT DB_ID(@p1)", t.Database).Scan(&id); err != nil {
		return false, err
	}
	conn, err := db.Conn(ctx) // USE only lasts for one session
	if err != nil {
		return false, err
	}
	defer conn.Close() //nolint:errcheck
	for _, s := range t.sqlServerStatements(password) {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			return false, fmt.Errorf("database %s: %w", t.Database, err)
		}
	}
	return !id.Valid, nil
}

// Drop deletes the database and its owner (the data is lost).
func (t Target) Drop(ctx context.Context, a Admin) error {
	var stmts []string
	dbname := ""
	switch t.Engine {
	case Oracle:
		stmts = []string{"DROP USER " + strings.ToUpper(t.Owner) + " CASCADE"}
	case SQLServer:
		d := bracket(t.Database)
		stmts = []string{
			"IF DB_ID(" + nliteral(t.Database) + ") IS NOT NULL BEGIN ALTER DATABASE " + d + " SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE " + d + " END",
			"IF EXISTS (SELECT 1 FROM sys.server_principals WHERE name = " + nliteral(t.Owner) + ") DROP LOGIN " + bracket(t.Owner),
		}
	default:
		dbname = "postgres"
		stmts = []string{"DROP DATABASE IF EXISTS " + pgQuote(t.Database) + " WITH (FORCE)", "DROP ROLE IF EXISTS " + pgQuote(t.Owner)}
	}
	db, err := t.open(ctx, a, dbname)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			if t.Engine == Oracle && strings.Contains(err.Error(), "ORA-01918") { // user does not exist
				continue
			}
			return err
		}
	}
	return nil
}
